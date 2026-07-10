package jwt_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
)

// testRSAKey is generated once for the whole package (RSA-2048
// generation is comparatively slow) and reused read-only by every
// test case below; none of the test cases mutate it.
var testRSAKey *rsa.PrivateKey

func TestMain(m *testing.M) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	testRSAKey = key
	m.Run()
}

func newTestClaims(exp int64) token.Claims {
	return token.Claims{
		Issuer:    "https://issuer.example",
		Subject:   "user-1",
		Audience:  "https://issuer.example",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: exp,
		Scope:     "openid profile",
	}
}

// TestSignVerify_RoundTrip covers traceability #5: a JWT signed by
// Signer verifies successfully with Verifier and the recovered Claims
// exactly match what was signed.
func TestSignVerify_RoundTrip(t *testing.T) {
	signer := jwt.NewSigner(testRSAKey, "test-kid")
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)

	claims := newTestClaims(time.Now().Add(1 * time.Hour).Unix())

	tokenString, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() unexpected error: %v", err)
	}
	if strings.Count(tokenString, ".") != 2 {
		t.Fatalf("Sign() token = %q, want a 3-segment compact JWT", tokenString)
	}

	got, err := verifier.Verify(tokenString)
	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if got != claims {
		t.Errorf("Verify() claims = %+v, want %+v", got, claims)
	}
}

// TestVerify_RejectsTamperedSignature covers the security requirement
// that a bit-flipped signature must not verify.
func TestVerify_RejectsTamperedSignature(t *testing.T) {
	signer := jwt.NewSigner(testRSAKey, "test-kid")
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)

	tokenString, err := signer.Sign(newTestClaims(time.Now().Add(1 * time.Hour).Unix()))
	if err != nil {
		t.Fatalf("Sign() unexpected error: %v", err)
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("Sign() token = %q, want a 3-segment compact JWT", tokenString)
	}
	// Flip the payload so the signature no longer matches it.
	tamperedPayload := base64.RawURLEncoding.EncodeToString(append(mustDecode(t, parts[1]), 'X'))
	tampered := parts[0] + "." + tamperedPayload + "." + parts[2]

	_, err = verifier.Verify(tampered)
	if err == nil {
		t.Fatal("Verify() on a tampered token succeeded, want an error")
	}
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("base64 decode %q: %v", s, err)
	}
	return b
}

// TestVerify_RejectsAlgNoneForgery covers the algorithm-confusion
// defense: a token whose header claims "alg":"none" (with no
// signature at all) must be rejected, never silently accepted.
func TestVerify_RejectsAlgNoneForgery(t *testing.T) {
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)

	header := map[string]string{"alg": "none", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadJSON, err := json.Marshal(newTestClaims(time.Now().Add(1 * time.Hour).Unix()))
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	forged := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + "."

	_, err = verifier.Verify(forged)
	if err == nil {
		t.Fatal("Verify() on an alg=none forgery succeeded, want it rejected")
	}
	if !isUnexpectedAlg(err) {
		t.Errorf("Verify() error = %v, want wrapping token.ErrUnexpectedAlg", err)
	}
}

// TestVerify_RejectsHS256DowngradeForgery covers the algorithm-confusion
// defense against an attacker who crafts an HS256 token (e.g. HMACing
// with the known public key as if it were a shared secret) and hopes
// the verifier will honor the attacker-chosen alg.
func TestVerify_RejectsHS256DowngradeForgery(t *testing.T) {
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadJSON, err := json.Marshal(newTestClaims(time.Now().Add(1 * time.Hour).Unix()))
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	forged := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("fake-hmac-signature"))

	_, err = verifier.Verify(forged)
	if err == nil {
		t.Fatal("Verify() on an HS256 downgrade forgery succeeded, want it rejected")
	}
	if !isUnexpectedAlg(err) {
		t.Errorf("Verify() error = %v, want wrapping token.ErrUnexpectedAlg", err)
	}
}

func isUnexpectedAlg(err error) bool {
	return errors.Is(err, token.ErrUnexpectedAlg)
}

// TestVerify_MalformedToken covers structurally invalid input (異).
func TestVerify_MalformedToken(t *testing.T) {
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "only two segments", input: "aaaa.bbbb"},
		{name: "four segments", input: "aaaa.bbbb.cccc.dddd"},
		{name: "non-base64 header", input: "!!!.bbbb.cccc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := verifier.Verify(tt.input); err == nil {
				t.Errorf("Verify(%q) succeeded, want an error", tt.input)
			}
		})
	}
}

// TestVerify_Expiry covers traceability #5 exp handling: valid tokens
// (正), expired tokens (異), and the exp boundary (境界) -- all via
// claims constructed with fixed exp offsets, never via sleep.
func TestVerify_Expiry(t *testing.T) {
	signer := jwt.NewSigner(testRSAKey, "test-kid")
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)

	tests := []struct {
		name    string
		expOffs time.Duration
		wantErr bool
	}{
		{name: "exp far in the future is valid", expOffs: 1 * time.Hour, wantErr: false},
		{name: "exp exactly now is still valid (boundary, inclusive)", expOffs: 0, wantErr: false},
		{name: "exp one second in the past is expired (boundary)", expOffs: -1 * time.Second, wantErr: true},
		{name: "exp far in the past is expired", expOffs: -1 * time.Hour, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := newTestClaims(time.Now().Add(tt.expOffs).Unix())
			tokenString, err := signer.Sign(claims)
			if err != nil {
				t.Fatalf("Sign() unexpected error: %v", err)
			}

			_, err = verifier.Verify(tokenString)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Verify() succeeded, want ErrTokenExpired")
				}
				if !errors.Is(err, token.ErrTokenExpired) {
					t.Errorf("Verify() error = %v, want wrapping token.ErrTokenExpired", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() unexpected error: %v", err)
			}
		})
	}
}

// TestJWKS_MatchesVerification covers traceability #9: the JWK
// published via KeyProvider.JWKS (n/e, base64url) reconstructs a
// public key that a fresh Verifier can use to validate a JWT signed
// by the corresponding private key.
func TestJWKS_MatchesVerification(t *testing.T) {
	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("ComputeKeyID() unexpected error: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)
	keyProvider := jwt.NewKeyProvider(&testRSAKey.PublicKey, kid)

	set := keyProvider.JWKS()
	if len(set.Keys) != 1 {
		t.Fatalf("JWKS().Keys = %d entries, want 1", len(set.Keys))
	}
	jwk := set.Keys[0]

	if jwk.Kty != "RSA" {
		t.Errorf("Kty = %q, want %q", jwk.Kty, "RSA")
	}
	if jwk.Use != "sig" {
		t.Errorf("Use = %q, want %q", jwk.Use, "sig")
	}
	if jwk.Alg != "RS256" {
		t.Errorf("Alg = %q, want %q", jwk.Alg, "RS256")
	}
	if jwk.Kid != kid {
		t.Errorf("Kid = %q, want %q", jwk.Kid, kid)
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		t.Fatalf("decode n: %v", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		t.Fatalf("decode e: %v", err)
	}
	reconstructed := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}

	// The reconstructed key must be numerically identical to the
	// original public key.
	if reconstructed.N.Cmp(testRSAKey.N) != 0 || reconstructed.E != testRSAKey.E {
		t.Fatal("public key reconstructed from JWK n/e does not match the original public key")
	}

	verifierFromJWK := jwt.NewVerifier(reconstructed)

	claims := newTestClaims(time.Now().Add(1 * time.Hour).Unix())
	tokenString, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() unexpected error: %v", err)
	}

	got, err := verifierFromJWK.Verify(tokenString)
	if err != nil {
		t.Fatalf("Verify() using the JWK-reconstructed key unexpected error: %v", err)
	}
	if got != claims {
		t.Errorf("Verify() claims = %+v, want %+v", got, claims)
	}
}

// TestComputeKeyID_IsDeterministicAndKeySpecific covers the RFC 7638
// thumbprint contract that ComputeKeyID relies on: the same key
// always yields the same kid, and different keys are (overwhelmingly)
// likely to yield different kids.
func TestComputeKeyID_IsDeterministicAndKeySpecific(t *testing.T) {
	kid1, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("ComputeKeyID() unexpected error: %v", err)
	}
	kid2, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("ComputeKeyID() unexpected error: %v", err)
	}
	if kid1 != kid2 {
		t.Errorf("ComputeKeyID() is not deterministic: %q != %q", kid1, kid2)
	}

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate second test key: %v", err)
	}
	kid3, err := jwt.ComputeKeyID(&otherKey.PublicKey)
	if err != nil {
		t.Fatalf("ComputeKeyID() unexpected error: %v", err)
	}
	if kid1 == kid3 {
		t.Error("ComputeKeyID() produced the same kid for two different keys, want distinct kids")
	}
}
