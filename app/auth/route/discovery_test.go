package route_test

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
)

type providerMetadataBody struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserInfoEndpoint                  string   `json:"userinfo_endpoint"`
	JWKSURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// TestDiscovery_Metadata covers traceability #8: REQUIRED and
// supported OIDC Discovery metadata.
func TestDiscovery_Metadata(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var meta providerMetadataBody
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode metadata: %v (body=%q)", err, rec.Body.String())
	}

	if meta.Issuer != testIssuer {
		t.Errorf("issuer = %q, want %q", meta.Issuer, testIssuer)
	}
	for name, got := range map[string]string{
		"authorization_endpoint": meta.AuthorizationEndpoint,
		"token_endpoint":         meta.TokenEndpoint,
		"userinfo_endpoint":      meta.UserInfoEndpoint,
		"jwks_uri":               meta.JWKSURI,
	} {
		if got == "" {
			t.Errorf("%s is empty, want a non-empty URL", name)
		}
	}

	if !reflect.DeepEqual(meta.ResponseTypesSupported, []string{"code"}) {
		t.Errorf("response_types_supported = %v, want [code]", meta.ResponseTypesSupported)
	}
	if !reflect.DeepEqual(meta.SubjectTypesSupported, []string{"public"}) {
		t.Errorf("subject_types_supported = %v, want [public]", meta.SubjectTypesSupported)
	}
	if !reflect.DeepEqual(meta.IDTokenSigningAlgValuesSupported, []string{"RS256"}) {
		t.Errorf("id_token_signing_alg_values_supported = %v, want [RS256]", meta.IDTokenSigningAlgValuesSupported)
	}
	if !reflect.DeepEqual(meta.CodeChallengeMethodsSupported, []string{"S256"}) {
		t.Errorf("code_challenge_methods_supported = %v, want [S256]", meta.CodeChallengeMethodsSupported)
	}
	if !reflect.DeepEqual(meta.GrantTypesSupported, []string{"authorization_code"}) {
		t.Errorf("grant_types_supported = %v, want [authorization_code]", meta.GrantTypesSupported)
	}
	if !reflect.DeepEqual(meta.TokenEndpointAuthMethodsSupported, []string{"none"}) {
		t.Errorf("token_endpoint_auth_methods_supported = %v, want [none] (public client, no client_secret support)", meta.TokenEndpointAuthMethodsSupported)
	}
	if len(meta.ScopesSupported) == 0 {
		t.Error("scopes_supported is empty, want at least openid")
	}
	if len(meta.ClaimsSupported) == 0 {
		t.Error("claims_supported is empty")
	}
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

// TestDiscovery_JWKS covers traceability #9: the JWK Set shape and
// that the published key can actually verify a JWT this server
// issues.
func TestDiscovery_JWKS(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
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
