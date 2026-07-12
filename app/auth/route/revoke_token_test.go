package route_test

import (
	"net/http"
	"testing"
)

// TestRevoke_RefreshToken_RevokesFamily covers RFC 7009: revoking a
// refresh token invalidates the whole family so subsequent refresh
// attempts fail.
func TestRevoke_RefreshToken_RevokesFamily(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid offline_access", "")

	rec := doRevoke(t, h, orig.RefreshToken, testClientID, "refresh_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	refresh := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if refresh.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", refresh.Code, http.StatusBadRequest, refresh.Body.String())
	}
	if got := decodeErrorBody(t, refresh).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
}

// TestRevoke_UnknownToken_Returns200 covers RFC 7009 §2.2: unknown
// tokens must still yield HTTP 200.
func TestRevoke_UnknownToken_Returns200(t *testing.T) {
	h := newTestHandler(t)

	rec := doRevoke(t, h, "never-issued-refresh-token", testClientID, "refresh_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestRevoke_AlreadyRevoked_Returns200 covers RFC 7009 §2.2: revoking
// an already-revoked token is idempotent and returns HTTP 200.
func TestRevoke_AlreadyRevoked_Returns200(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid offline_access", "")

	first := doRevoke(t, h, orig.RefreshToken, testClientID, "refresh_token")
	if first.Code != http.StatusOK {
		t.Fatalf("setup: first revoke status = %d, want %d (body=%q)", first.Code, http.StatusOK, first.Body.String())
	}

	second := doRevoke(t, h, orig.RefreshToken, testClientID, "refresh_token")
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", second.Code, http.StatusOK, second.Body.String())
	}
}

// TestRevoke_ClientMismatch_DoesNotRevoke covers client binding: a
// refresh token presented with a mismatched client_id must not be
// revoked, yet still returns HTTP 200 per RFC 7009.
func TestRevoke_ClientMismatch_DoesNotRevoke(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid offline_access", "")

	rec := doRevoke(t, h, orig.RefreshToken, testClientID2, "refresh_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	refresh := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if refresh.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (token must still be valid; body=%q)", refresh.Code, http.StatusOK, refresh.Body.String())
	}
}

// TestRevoke_AccessTokenHint_StillRevokesRefreshToken covers RFC 7009:
// when the hint does not match the token type the server ignores the
// hint and still revokes a refresh token.
func TestRevoke_AccessTokenHint_StillRevokesRefreshToken(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid offline_access", "")

	rec := doRevoke(t, h, orig.RefreshToken, testClientID, "access_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	refresh := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if refresh.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", refresh.Code, http.StatusBadRequest, refresh.Body.String())
	}
}

// TestRevoke_PublicClient_NoClientID_StillRevokes verifies that public
// clients (RFC 6749 2.1) may revoke their own refresh tokens without
// presenting client_id (RFC 7009 §2.1 optional for public clients).
func TestRevoke_PublicClient_NoClientID_StillRevokes(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid offline_access", "")

	rec := doRevoke(t, h, orig.RefreshToken, "", "refresh_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	refresh := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if refresh.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (token must be revoked; body=%q)", refresh.Code, http.StatusBadRequest, refresh.Body.String())
	}
}
