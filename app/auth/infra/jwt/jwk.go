// Package jwt provides the RSA/RS256 implementation of the ports
// declared by domain/token (Signer, Verifier, KeyProvider), built
// entirely from the Go standard library (crypto/rsa, crypto/sha256,
// encoding/base64, encoding/json).
package jwt

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// KeyProvider is the RSA implementation of token.KeyProvider: it
// publishes the authorization server's public signing key as a JWK
// (RFC 7517) for the /.well-known/jwks.json endpoint.
type KeyProvider struct {
	publicKey *rsa.PublicKey
	kid       string
}

// var _ token.KeyProvider = (*KeyProvider)(nil) verifies at compile
// time that KeyProvider satisfies the domain's KeyProvider port.
var _ token.KeyProvider = (*KeyProvider)(nil)

// NewKeyProvider builds a KeyProvider publishing publicKey under the
// given kid (key ID). kid should match the kid embedded in tokens
// produced by the corresponding Signer (see ComputeKeyID).
func NewKeyProvider(publicKey *rsa.PublicKey, kid string) *KeyProvider {
	return &KeyProvider{publicKey: publicKey, kid: kid}
}

// PublicJWK returns the authorization server's public signing key as
// a single JWK: kty=RSA, use=sig, alg=RS256, plus the base64url-encoded
// modulus (n) and exponent (e) (RFC 7517 / RFC 7518 6.3.1).
func (p *KeyProvider) PublicJWK() token.JWK {
	return token.JWK{
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		Kid: p.kid,
		N:   base64.RawURLEncoding.EncodeToString(p.publicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.publicKey.E)).Bytes()),
	}
}

// JWKS returns the JWK Set containing the authorization server's
// (single) public signing key.
func (p *KeyProvider) JWKS() token.JWKSet {
	return token.JWKSet{Keys: []token.JWK{p.PublicJWK()}}
}

// thumbprintMembers is the canonical member set for an RSA JWK
// thumbprint (RFC 7638 3.2): exactly {e, kty, n}, in lexicographic
// key order, with no insignificant whitespace.
type thumbprintMembers struct {
	E   string `json:"e"`
	Kty string `json:"kty"`
	N   string `json:"n"`
}

// ComputeKeyID derives a stable key ID for publicKey using the RFC
// 7638 JWK Thumbprint algorithm (SHA-256 over the canonical JSON
// member set), base64url-encoded. It is used both to tag JWTs (the
// header "kid") and to publish the matching JWK, so that a verifier
// consulting the JWKS endpoint can select the correct key.
func ComputeKeyID(publicKey *rsa.PublicKey) (string, error) {
	members := thumbprintMembers{
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
		Kty: "RSA",
		N:   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
	}
	canonical, err := json.Marshal(members)
	if err != nil {
		return "", fmt.Errorf("jwt: compute key id: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
