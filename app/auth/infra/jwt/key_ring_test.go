package jwt_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
)

func marshalPrivatePEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func marshalPublicPEM(t *testing.T, key *rsa.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// TestFileLoader_KeyRingPersistence covers ISSUE-036: a token signed
// with keys loaded from disk remains verifiable after reloading the
// same key file (simulating process restart).
func TestFileLoader_KeyRingPersistence(t *testing.T) {
	t.Parallel()

	activeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate active key: %v", err)
	}
	retiredKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate retired key: %v", err)
	}
	activeKid, err := jwt.ComputeKeyID(&activeKey.PublicKey)
	if err != nil {
		t.Fatalf("compute active kid: %v", err)
	}
	retiredKid, err := jwt.ComputeKeyID(&retiredKey.PublicKey)
	if err != nil {
		t.Fatalf("compute retired kid: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "signing-keys.json")
	ringJSON, err := json.Marshal(map[string]any{
		"active_kid": activeKid,
		"keys": []map[string]string{
			{"kid": activeKid, "private_key_pem": marshalPrivatePEM(t, activeKey)},
			{"kid": retiredKid, "public_key_pem": marshalPublicPEM(t, &retiredKey.PublicKey)},
		},
	})
	if err != nil {
		t.Fatalf("marshal key ring json: %v", err)
	}
	if err := os.WriteFile(path, ringJSON, 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	material, err := jwt.NewFileLoader(path).Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	ring, err := jwt.NewKeyRingFromMaterial(material)
	if err != nil {
		t.Fatalf("NewKeyRingFromMaterial() unexpected error: %v", err)
	}

	now := time.Now().Unix()
	signed, err := ring.Signer().Sign(token.Claims{
		Issuer:    "https://issuer.example",
		Subject:   "user-1",
		Audience:  "client-1",
		IssuedAt:  now,
		ExpiresAt: now + 3600,
	})
	if err != nil {
		t.Fatalf("Sign() unexpected error: %v", err)
	}

	reloaded, err := jwt.NewFileLoader(path).Load()
	if err != nil {
		t.Fatalf("reload Load() unexpected error: %v", err)
	}
	reloadedRing, err := jwt.NewKeyRingFromMaterial(reloaded)
	if err != nil {
		t.Fatalf("reload NewKeyRingFromMaterial() unexpected error: %v", err)
	}

	if _, err := reloadedRing.Verifier().Verify(signed); err != nil {
		t.Errorf("Verify() after reload: %v, want success", err)
	}

	set := reloadedRing.KeyProvider().JWKS()
	if len(set.Keys) != 2 {
		t.Fatalf("JWKS().Keys = %d, want 2 (active + retired)", len(set.Keys))
	}
}
