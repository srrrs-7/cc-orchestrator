//go:build integration

package route_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// TestToken_AuthorizationCode_IssuesRefreshToken covers R2: a client
// registered for grant_type=refresh_token (testClientID, see
// helpers_test.go) must receive a non-empty refresh_token alongside
// the access/ID tokens when exchanging an authorization code.
func TestToken_AuthorizationCode_IssuesRefreshToken(t *testing.T) {
	h := newTestHandler(t)

	got := issueTokens(t, h, "openid profile email", "")

	if got.RefreshToken == "" {
		t.Error("refresh_token is empty, want a non-empty refresh_token issued alongside access_token/id_token")
	}
}

// TestRefreshToken_Success_IssuesNewAccessAndIDToken covers R1
// (accept grant_type=refresh_token), R3 (access/ID token reissue: iss/
// sub/aud unchanged, a fresh iat, no nonce) and R4 (the refresh_token
// itself is rotated -- the response's refresh_token differs from the
// one just redeemed). It also re-covers R10's no-cache headers on the
// success path.
func TestRefreshToken_Success_IssuesNewAccessAndIDToken(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid profile email", "abc-nonce")
	origIDClaims := decodeJWTPayload(t, orig.IDToken)

	rec := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeTokenResponse(t, rec)

	if got.AccessToken == "" {
		t.Error("access_token is empty, want non-empty")
	}
	if got.IDToken == "" {
		t.Error("id_token is empty, want non-empty")
	}
	if got.RefreshToken == "" {
		t.Fatal("refresh_token is empty, want a rotated, non-empty refresh_token")
	}
	if got.RefreshToken == orig.RefreshToken {
		t.Error("refresh_token was not rotated: got the same value as the token that was just redeemed")
	}

	newIDClaims := decodeJWTPayload(t, got.IDToken)
	if newIDClaims.Issuer != origIDClaims.Issuer {
		t.Errorf("id_token iss = %q, want %q (unchanged across refresh)", newIDClaims.Issuer, origIDClaims.Issuer)
	}
	if newIDClaims.Subject != origIDClaims.Subject {
		t.Errorf("id_token sub = %q, want %q (unchanged across refresh)", newIDClaims.Subject, origIDClaims.Subject)
	}
	if newIDClaims.Audience != origIDClaims.Audience {
		t.Errorf("id_token aud = %q, want %q (unchanged across refresh)", newIDClaims.Audience, origIDClaims.Audience)
	}
	// A freshly minted ID token's iat must not be issued before the
	// original: compared with >= (not strict >) since both claims are
	// second-resolution Unix timestamps (RFC 7519 4.1.6) and an
	// in-process httptest round trip can complete within the same
	// wall-clock second, which would make a strict "must advance"
	// assertion flaky (testing.md "実時間 sleep に依存するテストを書かない"
	// -- this keeps the assertion sleep-independent).
	if newIDClaims.IssuedAt < origIDClaims.IssuedAt {
		t.Errorf("id_token iat = %d, want >= %d (a freshly issued ID token must not be issued before the original)", newIDClaims.IssuedAt, origIDClaims.IssuedAt)
	}
	if newIDClaims.ExpiresAt <= newIDClaims.IssuedAt {
		t.Errorf("id_token exp (%d) <= iat (%d), want exp after iat", newIDClaims.ExpiresAt, newIDClaims.IssuedAt)
	}
	if newIDClaims.Nonce != "" {
		t.Errorf("id_token nonce = %q, want empty (OIDC Core 12.2: a refreshed ID token must not echo the original authorization request's nonce)", newIDClaims.Nonce)
	}

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Errorf("Pragma = %q, want %q", got, "no-cache")
	}
}

// TestRefreshToken_Rotation_OldTokenRejected covers R4: once a refresh
// token has been rotated, re-presenting the exact token that was just
// redeemed must be rejected as invalid_grant.
func TestRefreshToken_Rotation_OldTokenRejected(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid", "")

	first := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if first.Code != http.StatusOK {
		t.Fatalf("setup: first refresh status = %d, want %d (body=%q)", first.Code, http.StatusOK, first.Body.String())
	}

	second := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if second.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", second.Code, http.StatusBadRequest, second.Body.String())
	}
	if got := decodeErrorBody(t, second).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
}

// TestRefreshToken_Reuse_RevokesFamily covers R5: re-presenting an
// already-rotated (consumed) refresh token is a reuse signal that
// must revoke the *entire* family -- not just reject the replayed
// token itself. This is proven by showing that the most recently
// rotated (otherwise still valid) refresh token is also rejected
// immediately afterward (RFC 9700 §4.14).
func TestRefreshToken_Reuse_RevokesFamily(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid", "")

	first := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if first.Code != http.StatusOK {
		t.Fatalf("setup: first refresh status = %d, want %d (body=%q)", first.Code, http.StatusOK, first.Body.String())
	}
	rotated := decodeTokenResponse(t, first)

	// Replay the already-consumed original token: detected as reuse.
	replay := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", replay.Code, http.StatusBadRequest, replay.Body.String())
	}
	if got := decodeErrorBody(t, replay).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}

	// The whole family must now be dead: even the currently-active,
	// legitimately rotated token must be rejected.
	afterRevocation := doRefreshToken(t, h, rotated.RefreshToken, testClientID, "")
	if afterRevocation.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (the freshest rotated token must be revoked as part of family revocation; body=%q)", afterRevocation.Code, http.StatusBadRequest, afterRevocation.Body.String())
	}
	if got := decodeErrorBody(t, afterRevocation).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
}

// TestRefreshToken_ClientMismatch_InvalidGrant covers R6: a refresh
// token issued to testClientID, presented while authenticating as a
// different, legitimately registered client (testClientID2), must be
// rejected as invalid_grant.
func TestRefreshToken_ClientMismatch_InvalidGrant(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid", "")

	rec := doRefreshToken(t, h, orig.RefreshToken, testClientID2, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := decodeErrorBody(t, rec).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
}

// TestRefreshToken_ScopeNarrower_OK covers R7: requesting a subset of
// the originally granted scope succeeds, drops the omitted scope
// value from the response, and that narrowing persists across a
// further rotation (a later refresh without an explicit scope must
// not be able to regain the dropped scope).
func TestRefreshToken_ScopeNarrower_OK(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid profile email", "")

	rec := doRefreshToken(t, h, orig.RefreshToken, testClientID, "openid profile")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeTokenResponse(t, rec)
	if strings.Contains(got.Scope, "email") {
		t.Errorf("scope = %q, want it to not contain email (narrowed away)", got.Scope)
	}
	if !strings.Contains(got.Scope, "openid") || !strings.Contains(got.Scope, "profile") {
		t.Errorf("scope = %q, want it to contain openid and profile", got.Scope)
	}

	second := doRefreshToken(t, h, got.RefreshToken, testClientID, "")
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", second.Code, http.StatusOK, second.Body.String())
	}
	got2 := decodeTokenResponse(t, second)
	if strings.Contains(got2.Scope, "email") {
		t.Errorf("scope = %q, want it to still not contain email (scope narrowing must persist across rotation)", got2.Scope)
	}
}

// TestRefreshToken_ScopeWiden_InvalidScope covers R7: requesting a
// scope broader than the originally granted one must be rejected as
// invalid_scope.
func TestRefreshToken_ScopeWiden_InvalidScope(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid profile", "")

	rec := doRefreshToken(t, h, orig.RefreshToken, testClientID, "openid profile email")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := decodeErrorBody(t, rec).Error; got != "invalid_scope" {
		t.Errorf("error = %q, want %q", got, "invalid_scope")
	}
}

// TestRefreshToken_Unknown_InvalidGrant covers R1's error path: a
// refresh_token value that was never issued must be rejected as
// invalid_grant.
func TestRefreshToken_Unknown_InvalidGrant(t *testing.T) {
	h := newTestHandler(t)

	rec := doRefreshToken(t, h, "this-refresh-token-was-never-issued", testClientID, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := decodeErrorBody(t, rec).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
}

// TestRefreshToken_EmptyToken_InvalidGrant covers R1's boundary case:
// an empty/missing refresh_token parameter can never match a
// persisted token, so it must be rejected the same way as an unknown
// token -- invalid_grant, not invalid_request (see
// service/authorization_service.go's refreshTokenGrant step 1, which
// treats req.RefreshToken == "" as refreshtoken.ErrNotFound). Also
// re-covers R10's no-cache headers on this specific error path.
func TestRefreshToken_EmptyToken_InvalidGrant(t *testing.T) {
	h := newTestHandler(t)

	rec := doRefreshToken(t, h, "", testClientID, "")
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

// TestRefreshToken_ErrorResponse_HasNoCacheHeaders covers R10: a
// grant_type=refresh_token error response must still carry
// Cache-Control: no-store and Pragma: no-cache (RFC 6749 5.1 MUST
// applies to every /token response, mirrors
// token_concurrency_test.go's TestToken_ErrorResponse_HasNoCacheHeaders
// for the authorization_code grant).
func TestRefreshToken_ErrorResponse_HasNoCacheHeaders(t *testing.T) {
	h := newTestHandler(t)

	rec := doRefreshToken(t, h, "does-not-exist", testClientID, "")
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

// TestRefreshToken_ConcurrentUse_ExactlyOneSucceeds covers the R4/R5
// atomicity requirement (SPEC-006 非機能要件 "単回使用の正しさ"): firing
// many concurrent /token requests with grant_type=refresh_token for
// the same refresh_token must yield exactly one 200 (a single
// successful rotation) and every other request must observe
// invalid_grant. Mirrors token_concurrency_test.go's
// TestToken_ConcurrentCodeReuse_ExactlyOneSucceeds for the
// authorization_code grant. Run with `go test -race` to also confirm
// the repository's Rotate critical section has no data race.
func TestRefreshToken_ConcurrentUse_ExactlyOneSucceeds(t *testing.T) {
	h := newTestHandler(t)
	orig := issueTokens(t, h, "openid", "")

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {orig.RefreshToken},
		"client_id":     {testClientID},
	}.Encode()

	const n = 20
	statuses := make([]int, n)
	errorCodes := make([]string, n)

	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start // all goroutines fire together, no ordering guarantee

			req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			// Each goroutine writes only to its own index i, so this is
			// race-free even though the slices are shared: there is no
			// overlapping access between goroutines.
			statuses[i] = rec.Code
			if rec.Code != http.StatusOK {
				var body struct {
					Error string `json:"error"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &body)
				errorCodes[i] = body.Error
			}
		}()
	}
	close(start)
	wg.Wait()

	// All result inspection and t.Error* calls happen here, back on
	// the test's own goroutine, after every worker has finished.
	var successCount, invalidGrantCount int
	for i := 0; i < n; i++ {
		switch statuses[i] {
		case http.StatusOK:
			successCount++
		case http.StatusBadRequest:
			if errorCodes[i] == "invalid_grant" {
				invalidGrantCount++
			} else {
				t.Errorf("request %d: status %d with unexpected error %q, want invalid_grant", i, statuses[i], errorCodes[i])
			}
		default:
			t.Errorf("request %d: unexpected status %d", i, statuses[i])
		}
	}

	if successCount != 1 {
		t.Errorf("successCount = %d, want exactly 1 (rotation must be atomic under concurrency)", successCount)
	}
	if invalidGrantCount != n-1 {
		t.Errorf("invalidGrantCount = %d, want %d", invalidGrantCount, n-1)
	}
}
