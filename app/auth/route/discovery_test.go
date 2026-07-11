// Offline (untagged) discovery tests. These tests only exercise the
// OIDC metadata and JWKS endpoints in ways that never call the
// authorization or user-info service methods that touch repositories.
// They run as part of the default `make test` / `make check`.
//
// TestDiscovery_JWKS -- which does a full authorize+token exchange to
// verify the JWKS key -- requires a live DB and lives in
// discovery_integration_test.go (//go:build integration).
package route_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
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
// supported OIDC Discovery metadata. Uses newDiscoveryTestHandler
// (offline, no DB) because the discovery endpoint only reads the
// issuer string and keyProvider -- no repository state is required.
func TestDiscovery_Metadata(t *testing.T) {
	h := newDiscoveryTestHandler(t)

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
	if !reflect.DeepEqual(meta.GrantTypesSupported, []string{"authorization_code", "refresh_token"}) {
		t.Errorf("grant_types_supported = %v, want [authorization_code refresh_token] (SPEC-006 R9)", meta.GrantTypesSupported)
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
