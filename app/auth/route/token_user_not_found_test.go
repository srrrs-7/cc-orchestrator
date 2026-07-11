//go:build integration

// Regression coverage for ISSUE-019 課題3: when a grant (refresh_token
// / authorization_code) resolves to a valid, unexpired token/code but
// the resource owner it names has since disappeared from the user
// repository (e.g. deleted after the grant was issued), /token must
// respond with invalid_grant (HTTP 400) rather than the previous
// generic server_error (HTTP 500). See route/response.go's
// tokenErrorCode (the errors.Is(err, user.ErrNotFound) case, shared by
// both grants) and
// docs/issues/20260710-019-auth-refresh-token-deferred-hardening.md
// 課題3 for the full history.
//
// SPEC-011: The previous implementation used a test-local
// removableUserRepository (an in-memory stub with a Delete method).
// This integration version uses a real Postgres DB and performs an SQL
// DELETE on the users table to simulate the owner disappearing --
// identical semantic, no in-memory store required.
package route_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
)

// TestRefreshToken_UserNotFound_InvalidGrant covers ISSUE-019 課題3's
// refresh_token path: a refresh token that is itself valid (not
// expired/consumed/reused, correctly bound to the requesting client)
// but whose owner has disappeared from the user repository must be
// rejected as invalid_grant (400), not the previous generic
// server_error (500).
func TestRefreshToken_UserNotFound_InvalidGrant(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	h := newTestHandlerWithDB(t, db)

	// Issue a full token set (including a refresh token) while the
	// resource owner still exists.
	orig := issueTokens(t, h, "openid", "")

	// Simulate the owner disappearing after the grant was issued:
	// DELETE the user row directly via the shared DB handle.
	_, err := db.ExecContext(context.Background(),
		"DELETE FROM users WHERE id = $1", testUserID)
	if err != nil {
		t.Fatalf("DELETE user: %v", err)
	}

	rec := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	got := decodeErrorBody(t, rec)
	if got.Error != "invalid_grant" {
		t.Errorf("error = %q, want %q", got.Error, "invalid_grant")
	}

	// The description must not leak that the failure was specifically
	// "user not found": it must read like any other invalid_grant
	// cause, never mentioning the user (ISSUE-019 課題3: no
	// user-existence oracle via /token).
	if strings.Contains(strings.ToLower(got.ErrorDescription), "user") {
		t.Errorf("error_description = %q, must not mention \"user\" (would leak user-existence information)", got.ErrorDescription)
	}
	if want := "the authorization grant is invalid, expired, or already used"; got.ErrorDescription != want {
		t.Errorf("error_description = %q, want %q", got.ErrorDescription, want)
	}
}

// TestToken_AuthorizationCode_UserNotFound_InvalidGrant covers
// ISSUE-019 課題3's authorization_code path (the same tokenErrorCode
// fix covers both grants, sharing the errors.Is(err, user.ErrNotFound)
// case in route/response.go): an authorization code that is itself
// valid (correct PKCE, not expired/consumed) but whose owner has
// disappeared from the user repository must be rejected as
// invalid_grant (400), not server_error (500).
func TestToken_AuthorizationCode_UserNotFound_InvalidGrant(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	h := newTestHandlerWithDB(t, db)

	verifier := strings.Repeat("A", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifier), "openid", "")

	// Simulate the owner disappearing after the code was issued but
	// before it is exchanged.
	_, err := db.ExecContext(context.Background(),
		"DELETE FROM users WHERE id = $1", testUserID)
	if err != nil {
		t.Fatalf("DELETE user: %v", err)
	}

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifier},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	got := decodeErrorBody(t, rec)
	if got.Error != "invalid_grant" {
		t.Errorf("error = %q, want %q", got.Error, "invalid_grant")
	}

	if strings.Contains(strings.ToLower(got.ErrorDescription), "user") {
		t.Errorf("error_description = %q, must not mention \"user\" (would leak user-existence information)", got.ErrorDescription)
	}
	if want := "the authorization grant is invalid, expired, or already used"; got.ErrorDescription != want {
		t.Errorf("error_description = %q, want %q", got.ErrorDescription, want)
	}
}
