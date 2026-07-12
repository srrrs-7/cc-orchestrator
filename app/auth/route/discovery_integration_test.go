// DB-backed coverage for TestDiscovery_JWKS, which exercises the full
// authorize → token flow to verify that the JWKS-published key can
// actually verify a token this server issues. Kept in its own file,
// separate from discovery_test.go, because issuing tokens requires a
// real Postgres test DB (newTestHandler) rather than the nil-repo
// discovery-only handler.
package route_test

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
)

// TestDiscovery_JWKS covers traceability #9: the JWK Set shape and
// that the published key can actually verify a JWT this server issues.
//
// This test requires a real Postgres test DB (it does a full
// authorize+token exchange to obtain a real JWT), so it is kept
// separate from the nil-repo TestDiscovery_Metadata in
// discovery_test.go.
func TestDiscovery_JWKS(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	type jwkBody struct {
		Kty string `json:"kty"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	type jwksBody struct {
		Keys []jwkBody `json:"keys"`
	}

	var set jwksBody
	if err := json.Unmarshal(rec.Body.Bytes(), &set); err != nil {
		t.Fatalf("decode jwks: %v (body=%q)", err, rec.Body.String())
	}
	if len(set.Keys) != 1 {
		t.Fatalf("keys = %d entries, want 1", len(set.Keys))
	}
	jwk := set.Keys[0]
	if jwk.Kty != "RSA" || jwk.Use != "sig" || jwk.Alg != "RS256" || jwk.Kid == "" {
		t.Fatalf("jwk = %+v, want kty=RSA use=sig alg=RS256 with a non-empty kid", jwk)
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		t.Fatalf("decode n: %v", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		t.Fatalf("decode e: %v", err)
	}
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(new(big.Int).SetBytes(eBytes).Int64())}
	verifier := jwt.NewVerifier(pub)

	verifierStr := strings.Repeat("A", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifierStr), "openid", "")
	tokenRec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifierStr},
	})
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("setup: token exchange status = %d, want %d (body=%q)", tokenRec.Code, http.StatusOK, tokenRec.Body.String())
	}
	tokenResp := decodeTokenResponse(t, tokenRec)

	if _, err := verifier.Verify(tokenResp.AccessToken); err != nil {
		t.Errorf("Verify() access_token using the JWKS-published key: %v, want success", err)
	}
}
