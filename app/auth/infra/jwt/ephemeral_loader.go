package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// EphemeralLoader implements token.KeyRingLoader by generating a fresh
// RSA-2048 key pair in memory at each Load call. It is used when
// SIGNING_KEYS_FILE is not set (local development / tests), producing
// a key ring that does not survive process restart. This is safe for
// dev/test but must not be used in production.
type EphemeralLoader struct{}

var _ token.KeyRingLoader = (*EphemeralLoader)(nil)

// NewEphemeralLoader returns an EphemeralLoader.
func NewEphemeralLoader() *EphemeralLoader { return &EphemeralLoader{} }

// Load generates a fresh RSA-2048 private key, derives its kid via
// ComputeKeyID, and returns a single-entry SigningMaterial.
func (e *EphemeralLoader) Load() (token.SigningMaterial, error) {
	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return token.SigningMaterial{}, fmt.Errorf("jwt: ephemeral loader: generate key: %w", err)
	}
	kid, err := ComputeKeyID(&key.PublicKey)
	if err != nil {
		return token.SigningMaterial{}, fmt.Errorf("jwt: ephemeral loader: compute key id: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return token.SigningMaterial{
		ActiveKid: kid,
		Keys:      []token.KeyEntry{{Kid: kid, PrivateKeyPEM: string(privPEM)}},
	}, nil
}

// rsaKeyBits is the RSA key size used for both ephemeral generation and
// key ring generation. 2048 bits meets RFC 7518's RS256 requirements
// and NIST SP 800-57 Part 1's minimum for deployments through 2030.
const rsaKeyBits = 2048
