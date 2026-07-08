package jwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// jwtHeader is the JOSE header for the compact JWTs this
// authorization server issues. It is fixed to RS256/JWT; kid
// identifies which published JWK a verifier should use.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

// Signer is the RSA implementation of token.Signer. It produces
// compact JWTs signed with RSASSA-PKCS1-v1_5 using SHA-256 (RS256,
// RFC 7518 3.3), built entirely from crypto/rsa and crypto/sha256.
type Signer struct {
	privateKey *rsa.PrivateKey
	kid        string
}

// var _ token.Signer = (*Signer)(nil) verifies at compile time that
// Signer satisfies the domain's Signer port.
var _ token.Signer = (*Signer)(nil)

// NewSigner builds a Signer using privateKey, tagging every JWT it
// produces with the given kid (key ID; see ComputeKeyID).
func NewSigner(privateKey *rsa.PrivateKey, kid string) *Signer {
	return &Signer{privateKey: privateKey, kid: kid}
}

// Sign encodes claims as a compact JWT:
// base64url(header) + "." + base64url(payload) + "." + base64url(signature),
// where signature = RSASSA-PKCS1-v1_5(SHA-256(header "." payload)).
func (s *Signer) Sign(claims token.Claims) (string, error) {
	headerJSON, err := json.Marshal(jwtHeader{Alg: "RS256", Typ: "JWT", Kid: s.kid})
	if err != nil {
		return "", fmt.Errorf("jwt: sign: encode header: %w", err)
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("jwt: sign: encode payload: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON)

	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("jwt: sign: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
