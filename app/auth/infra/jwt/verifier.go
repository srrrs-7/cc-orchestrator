package jwt

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// Verifier is the RSA implementation of token.Verifier. It verifies
// compact JWTs signed with RS256 and rejects any other "alg" -- most
// importantly "none" and HMAC ("HS256"/...) -- so that an attacker
// cannot perform an algorithm-confusion downgrade attack against a
// verifier that would otherwise accept whatever alg a token claims.
type Verifier struct {
	publicKey *rsa.PublicKey
}

// var _ token.Verifier = (*Verifier)(nil) verifies at compile time
// that Verifier satisfies the domain's Verifier port.
var _ token.Verifier = (*Verifier)(nil)

// NewVerifier builds a Verifier that checks signatures against
// publicKey.
func NewVerifier(publicKey *rsa.PublicKey) *Verifier {
	return &Verifier{publicKey: publicKey}
}

// Verify parses tokenString as a compact JWT, checks its header
// "alg" is exactly "RS256", verifies its RSASSA-PKCS1-v1_5/SHA-256
// signature against the Verifier's public key, checks it has not
// expired, and returns the decoded Claims.
func (v *Verifier) Verify(tokenString string) (token.Claims, error) {
	header, parts, err := decodeJWTHeader(tokenString)
	if err != nil {
		return token.Claims{}, err
	}
	// Strictly require RS256. This is the algorithm-confusion defense:
	// never trust the token to declare an algorithm the verifier will
	// then blindly honor.
	if header.Alg != "RS256" {
		return token.Claims{}, fmt.Errorf("jwt: verify: alg %q: %w", header.Alg, token.ErrUnexpectedAlg)
	}

	return verifyRS256(parts, v.publicKey)
}

// decodeJWTHeader is a package-private helper shared between Verifier
// and MultiKeyVerifier. It splits the compact JWT and decodes its JOSE
// header, returning the header and the three raw parts on success.
func decodeJWTHeader(tokenString string) (jwtHeader, [3]string, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return jwtHeader{}, [3]string{}, fmt.Errorf("jwt: verify: %w", token.ErrInvalidToken)
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtHeader{}, [3]string{}, fmt.Errorf("jwt: verify: decode header: %w", token.ErrInvalidToken)
	}
	var h jwtHeader
	if err := json.Unmarshal(headerJSON, &h); err != nil {
		return jwtHeader{}, [3]string{}, fmt.Errorf("jwt: verify: decode header: %w", token.ErrInvalidToken)
	}
	return h, [3]string{parts[0], parts[1], parts[2]}, nil
}

// verifyRS256 checks the RSASSA-PKCS1-v1_5/SHA-256 signature and
// expiry of a compact JWT whose three parts have already been split.
func verifyRS256(parts [3]string, publicKey *rsa.PublicKey) (token.Claims, error) {
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return token.Claims{}, fmt.Errorf("jwt: verify: decode signature: %w", token.ErrInvalidToken)
	}

	signingInput := parts[0] + "." + parts[1]
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, sum[:], signature); err != nil {
		return token.Claims{}, fmt.Errorf("jwt: verify: %w", token.ErrSignatureInvalid)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return token.Claims{}, fmt.Errorf("jwt: verify: decode payload: %w", token.ErrInvalidToken)
	}
	var claims token.Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return token.Claims{}, fmt.Errorf("jwt: verify: decode payload: %w", token.ErrInvalidToken)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return token.Claims{}, fmt.Errorf("jwt: verify: %w", token.ErrTokenExpired)
	}

	return claims, nil
}
