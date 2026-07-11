// Offline (untagged) security regression tests. These tests verify
// error paths that never reach the repository layer, so they can run
// without a live DB as part of the default `make test` / `make check`.
//
// Test handler rationale:
//
//   - TestUserInfo_ValidSignatureWrongAudOrIssuer_Unauthorized: JWT
//     validation (iss/aud check) fails before any userRepo access, so
//     newDiscoveryTestHandler (nil userRepo) is sufficient.
//
//   - TestToken_ErrorResponse_HasNoCacheHeaders: the /token error path
//     needs a valid client lookup followed by an auth code not-found.
//     newTokenErrorTestHandler provides a minimal stubClientOnlyRepo
//     (returns the test client) and alwaysNotFoundAuthCodeRepo (returns
//     ErrNotFound), without requiring a real DB.
package route_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
)

// TestUserInfo_ValidSignatureWrongAudOrIssuer_Unauthorized covers the
// review-spec gap: token.Verifier only checks signature + exp, so
// UserInfoService itself is what MUST reject a token that verifies
// cleanly (correct signature, correct RS256 alg, not expired) but
// carries an "iss"/"aud" that does not match this authorization
// server's issuer/audience design. Both forged tokens below are
// signed with the exact same key material the running handler
// verifies against, so a failure here can only be explained by a
// missing/broken iss or aud check in UserInfoService.UserInfo -- not
// by signature verification.
//
// Uses newDiscoveryTestHandler: the JWT iss/aud validation fails
// before userRepo is accessed, so nil repos suffice.
func TestUserInfo_ValidSignatureWrongAudOrIssuer_Unauthorized(t *testing.T) {
	h := newDiscoveryTestHandler(t)

	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("setup ComputeKeyID() unexpected error: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)

	tests := []struct {
		name     string
		issuer   string
		audience string
	}{
		{name: "wrong issuer, correct audience", issuer: "https://attacker.example", audience: testIssuer},
		{name: "correct issuer, wrong audience", issuer: testIssuer, audience: "https://some-other-resource-server.example"},
		{name: "wrong issuer and wrong audience", issuer: "https://attacker.example", audience: "https://some-other-resource-server.example"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := token.NewAccessTokenClaims(tt.issuer, testUserID, tt.audience, "openid profile email")
			forged, err := signer.Sign(claims)
			if err != nil {
				t.Fatalf("setup Sign() unexpected error: %v", err)
			}

			rec := doUserInfo(t, h, forged)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}
}

// TestToken_ErrorResponse_HasNoCacheHeaders covers traceability #4's
// no-cache requirement on the error path (RFC 6749 5.1 MUST applies
// to every /token response, not only successful ones): both
// Cache-Control: no-store and Pragma: no-cache must be present even
// when the request is rejected.
//
// Uses newTokenErrorTestHandler: the test client exists (stubClientOnlyRepo)
// and the submitted code "does-not-exist" is never found
// (alwaysNotFoundAuthCodeRepo → invalid_grant). No live DB required.
func TestToken_ErrorResponse_HasNoCacheHeaders(t *testing.T) {
	h := newTokenErrorTestHandler(t)

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"does-not-exist"},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {strings.Repeat("A", 43)},
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := decodeErrorBody(t, rec).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Errorf("Pragma = %q, want %q", got, "no-cache")
	}
}
