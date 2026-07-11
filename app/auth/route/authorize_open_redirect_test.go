package route_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// attackerRedirectURI is a well-formed http(s) URI that is never
// registered for any client seeded by newTestHandler. It stands in
// for an attacker-controlled callback host in the open-redirect
// regression cases below (ISSUE-004).
const attackerRedirectURI = "https://attacker.example/steal-tokens"

// TestAuthorize_UnverifiedError_NeverRedirectsToAttackerURI is the
// ISSUE-004 regression covering the invariant of RFC 6749 4.1.2.1:
// /authorize errors that occur *before* client_id/redirect_uri are
// confirmed valid MUST be reported directly, never via a 302
// redirect -- otherwise this endpoint could be abused as an open
// redirector to an unregistered/attacker-controlled URI.
//
// This exercises route/authorize_handler.go's data-flow fix: the
// handler now passes service.AuthorizeResult.RedirectURI (which
// AuthorizationService.Authorize leaves as the empty string for every
// error above its "client_id/redirect_uri are now verified" point) to
// writeAuthorizeError, instead of the raw, attacker-suppliable
// req.RedirectURI query parameter. Every case below supplies
// attackerRedirectURI (or another unverifiable value) as
// redirect_uri/client_id and asserts no redirect to it ever happens.
func TestAuthorize_UnverifiedError_NeverRedirectsToAttackerURI(t *testing.T) {
	h := newTestHandler(t)

	tests := []struct {
		name          string
		clientID      string
		redirectURI   string
		wantErrorCode string
	}{
		{
			name:          "registered client, unregistered (attacker-controlled) redirect_uri",
			clientID:      testClientID,
			redirectURI:   attackerRedirectURI,
			wantErrorCode: "invalid_request", // client.ErrRedirectURIMismatch
		},
		{
			name:          "registered client, malformed redirect_uri",
			clientID:      testClientID,
			redirectURI:   "not-a-uri",
			wantErrorCode: "invalid_request", // client.ErrInvalidRedirectURI
		},
		{
			name:          "unknown client_id, attacker-controlled redirect_uri",
			clientID:      "unknown-client",
			redirectURI:   attackerRedirectURI,
			wantErrorCode: "invalid_client", // client.ErrNotFound
		},
		{
			name:          "empty client_id, attacker-controlled redirect_uri",
			clientID:      "",
			redirectURI:   attackerRedirectURI,
			wantErrorCode: "invalid_request", // client.ErrInvalidClientID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doAuthorize(t, h, url.Values{
				"response_type": {"code"},
				"client_id":     {tt.clientID},
				"redirect_uri":  {tt.redirectURI},
				"scope":         {"openid"},
				"state":         {"attacker-state"},
			})

			if rec.Code == http.StatusFound {
				t.Fatalf("status = %d (redirect), want a direct non-redirect error response (body=%q)", rec.Code, rec.Body.String())
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}

			loc := rec.Header().Get("Location")
			if loc != "" {
				t.Errorf("Location header = %q, want empty (unverified redirect_uri/client_id must never be redirected to)", loc)
			}
			if strings.Contains(loc, "attacker.example") {
				t.Errorf("Location header = %q, must not contain the attacker-controlled redirect_uri", loc)
			}

			if got := decodeErrorBody(t, rec).Error; got != tt.wantErrorCode {
				t.Errorf("error = %q, want %q", got, tt.wantErrorCode)
			}
		})
	}
}

// TestAuthorize_VerifiedRedirectURI_RedirectsToRegisteredURI_OnUnsupportedResponseType
// is the positive counterpart of the above: once client_id and
// redirect_uri are confirmed valid, a subsequent OAuth error
// (response_type=token is unsupported; this authorization server only
// implements response_type=code, RFC 6749 4.1.2.1) MUST still be
// reported via redirect, and specifically to the client's registered
// redirect_uri -- never to an attacker-suppliable value. This guards
// against a fix for the open-redirect invariant regressing the normal
// (and required) redirect-on-error behavior for already-verified
// requests.
func TestAuthorize_VerifiedRedirectURI_RedirectsToRegisteredURI_OnUnsupportedResponseType(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	rec := doAuthorize(t, h, url.Values{
		"response_type":         {"token"}, // unsupported: only response_type=code is implemented
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"s-redirect"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	})

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (verified client/redirect_uri errors are reported via redirect; body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location header: %v", err)
	}

	// The redirect target must be exactly the client's registered
	// redirect_uri (scheme+host+path, query stripped) -- never an
	// attacker-suppliable value.
	gotBase := (&url.URL{Scheme: loc.Scheme, Host: loc.Host, Path: loc.Path}).String()
	if gotBase != testRedirectURI {
		t.Errorf("redirect target = %q, want registered redirect_uri %q", gotBase, testRedirectURI)
	}
	if got := loc.Query().Get("error"); got != "unsupported_response_type" {
		t.Errorf("redirect error = %q, want %q", got, "unsupported_response_type")
	}
	if got := loc.Query().Get("state"); got != "s-redirect" {
		t.Errorf("redirect state = %q, want %q", got, "s-redirect")
	}
}
