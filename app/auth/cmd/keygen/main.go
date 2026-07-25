// Command keygen generates a new RSA-2048 signing key ring and writes
// it as a JSON file suitable for consumption by infra/jwt.FileLoader.
//
// Usage:
//
//	go run ./cmd/keygen [-out <path>]
//
// The output file path defaults to ../../.secrets/auth-signing-keys.json
// (relative to app/auth), which resolves to .secrets/auth-signing-keys.json
// at the repository root. That directory is gitignored; never commit the
// generated file.
//
// To rotate: run keygen with the --rotate flag and the current key file.
// The current active key is demoted to a verify-only retired key (public
// key only), and a fresh key becomes the new active_kid.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
)

func main() {
	if err := run(); err != nil {
		slog.Error("keygen: fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("out", defaultOut(), "output file path for the key ring JSON")
	rotate := flag.Bool("rotate", false, "rotate: demote the current active key to verify-only and generate a new active key")
	flag.Parse()

	if *rotate {
		return rotateKeyFile(*out)
	}
	return generateNewKeyFile(*out)
}

// defaultOut resolves the default output path relative to this
// binary's source directory (app/auth), landing at the repo-root
// .secrets/ directory.
func defaultOut() string {
	// When running from the toolchain container at /workspace/app/auth,
	// the .secrets directory is two levels up at /workspace/.secrets.
	// When running from the host from the app/auth directory, ../../.secrets
	// also lands at the repo root. Callers may override with --out.
	return filepath.Join("..", "..", ".secrets", "auth-signing-keys.json")
}

// generateNewKeyFile creates a fresh single-key key ring at path.
// It fails if the file already exists (use --rotate to rotate).
func generateNewKeyFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("keygen: %q already exists; use --rotate to add a new key", path)
	}

	priv, kid, err := generateKey()
	if err != nil {
		return err
	}

	ring := keyRingFile{
		ActiveKid: kid,
		Keys:      []keyEntryFile{{Kid: kid, PrivateKeyPEM: encodePrivatePEM(priv)}},
	}
	return writeKeyFile(path, ring)
}

// rotateKeyFile reads the existing key ring at path, demotes the
// current active key to a verify-only retired entry (dropping its
// private key PEM), and appends a newly generated active key.
func rotateKeyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("keygen: rotate: read %q: %w", path, err)
	}
	var existing keyRingFile
	if err := json.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("keygen: rotate: parse %q: %w", path, err)
	}

	priv, newKid, err := generateKey()
	if err != nil {
		return err
	}

	// Demote existing keys: drop private_key_pem, keep public_key_pem.
	retired := make([]keyEntryFile, 0, len(existing.Keys))
	for _, k := range existing.Keys {
		if k.PrivateKeyPEM == "" {
			retired = append(retired, k)
			continue
		}
		// Derive public PEM from the stored private PEM.
		pubPEM, err := extractPublicPEM(k.PrivateKeyPEM)
		if err != nil {
			slog.Warn("keygen: rotate: cannot demote key; skipping", "kid", k.Kid, "error", err)
			continue
		}
		retired = append(retired, keyEntryFile{Kid: k.Kid, PublicKeyPEM: pubPEM})
	}

	ring := keyRingFile{
		ActiveKid: newKid,
		Keys:      append([]keyEntryFile{{Kid: newKid, PrivateKeyPEM: encodePrivatePEM(priv)}}, retired...),
	}
	slog.Info("keygen: rotate: demoted old keys", "count", len(retired), "new_kid", newKid)
	return writeKeyFile(path, ring)
}

// generateKey produces a fresh RSA-2048 private key and derives its kid.
func generateKey() (*rsa.PrivateKey, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", fmt.Errorf("keygen: generate key: %w", err)
	}
	kid, err := jwt.ComputeKeyID(&priv.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("keygen: compute kid: %w", err)
	}
	return priv, kid, nil
}

// encodePrivatePEM encodes an RSA private key as a PKCS#1 PEM string.
func encodePrivatePEM(priv *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}))
}

// extractPublicPEM derives a PKIX PEM public key from a PKCS#1 PEM
// private key string.
func extractPublicPEM(privPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return "", fmt.Errorf("no PEM block found")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})), nil
}

func writeKeyFile(path string, ring keyRingFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("keygen: mkdir %q: %w", filepath.Dir(path), err)
	}
	out, err := json.MarshalIndent(ring, "", "  ")
	if err != nil {
		return fmt.Errorf("keygen: marshal: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("keygen: write %q: %w", path, err)
	}
	slog.Info("keygen: key ring written", "path", path, "active_kid", ring.ActiveKid, "total_keys", len(ring.Keys))
	return nil
}

// keyRingFile mirrors infra/jwt.fileKeyRing (duplicated here to avoid
// importing an internal package from a sibling command).
type keyRingFile struct {
	ActiveKid string         `json:"active_kid"`
	Keys      []keyEntryFile `json:"keys"`
}

type keyEntryFile struct {
	Kid           string `json:"kid"`
	PrivateKeyPEM string `json:"private_key_pem,omitempty"`
	PublicKeyPEM  string `json:"public_key_pem,omitempty"`
}
