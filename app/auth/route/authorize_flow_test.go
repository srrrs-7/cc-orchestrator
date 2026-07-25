package route_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestAuthorize_Unauthenticated_RedirectsToLogin ensures /authorize
// sends unauthenticated users to the IdP login page (ISSUE-031).
func TestAuthorize_Unauthenticated_RedirectsToLogin(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != testIssuer+"/login" {
		t.Errorf("Location = %q, want %q", got, testIssuer+"/login")
	}
}

// TestAuthorizeTokenUserInfoFlow_Success drives authorize -> token ->
// userinfo end-to-end. It covers traceability #1 (authorization
// request), #2 (opaque single-use code), #3 (PKCE S256 success), #4
// (token success shape + Cache-Control: no-store + Pragma: no-cache),
// #5 (RS256 JWTs), #6 (ID Token REQUIRED claims + nonce reflection)
// and #7 (UserInfo sub + scope-gated claims). It also asserts the
// access token's audience design (aud = API resource identifier per
// ISSUE-037, distinct from issuer).
func TestAuthorizeTokenUserInfoFlow_Success(t *testing.T) {
	h := newTestHandler(t)

	verifier := strings.Repeat("A", 43)
	challenge := pkceChallenge(verifier)

	authRec := doAuthorize(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid profile email"},
		"state":                 {"xyz-state"},
		"nonce":                 {"abc-nonce"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	})
	if authRec.Code != http.StatusFound {
		t.Fatalf("authorize status = %d, want %d (body=%q)", authRec.Code, http.StatusFound, authRec.Body.String())
	}
	loc, err := url.Parse(authRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location header: %v", err)
	}
	if got := loc.Query().Get("state"); got != "xyz-state" {
		t.Errorf("redirect state = %q, want %q", got, "xyz-state")
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("redirect code is empty, want non-empty authorization code")
	}

	tokenRec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifier},
	})
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token status = %d, want %d (body=%q)", tokenRec.Code, http.StatusOK, tokenRec.Body.String())
	}
	if got := tokenRec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := tokenRec.Header().Get("Pragma"); got != "no-cache" {
		t.Errorf("Pragma = %q, want %q (RFC 6749 5.1 MUST, for HTTP/1.0 caches)", got, "no-cache")
	}

	tokenResp := decodeTokenResponse(t, tokenRec)
	if tokenResp.AccessToken == "" {
		t.Error("access_token is empty, want non-empty")
	}
	if tokenResp.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want %q", tokenResp.TokenType, "Bearer")
	}
	if tokenResp.IDToken == "" {
		t.Error("id_token is empty, want non-empty")
	}
	if !strings.Contains(tokenResp.Scope, "openid") {
		t.Errorf("scope = %q, want to contain openid", tokenResp.Scope)
	}

	accessClaims := decodeJWTPayload(t, tokenResp.AccessToken)
	if accessClaims.Audience != testAPIAudience {
		t.Errorf("access_token aud = %q, want %q (ISSUE-037: aud = API resource identifier, not issuer)", accessClaims.Audience, testAPIAudience)
	}
	if accessClaims.Issuer != testIssuer {
		t.Errorf("access_token iss = %q, want %q", accessClaims.Issuer, testIssuer)
	}

	idClaims := decodeJWTPayload(t, tokenResp.IDToken)
	if idClaims.Issuer != testIssuer {
		t.Errorf("id_token iss = %q, want %q", idClaims.Issuer, testIssuer)
	}
	if idClaims.Subject != testUserID {
		t.Errorf("id_token sub = %q, want %q", idClaims.Subject, testUserID)
	}
	if idClaims.Audience != testClientID {
		t.Errorf("id_token aud = %q, want %q (client_id)", idClaims.Audience, testClientID)
	}
	if idClaims.IssuedAt == 0 {
		t.Error("id_token iat is zero, want set")
	}
	if idClaims.ExpiresAt <= idClaims.IssuedAt {
		t.Errorf("id_token exp (%d) <= iat (%d), want exp after iat", idClaims.ExpiresAt, idClaims.IssuedAt)
	}
	if idClaims.Nonce != "abc-nonce" {
		t.Errorf("id_token nonce = %q, want %q (echoed from the authorization request)", idClaims.Nonce, "abc-nonce")
	}

	userInfoRec := doUserInfo(t, h, tokenResp.AccessToken)
	if userInfoRec.Code != http.StatusOK {
		t.Fatalf("userinfo status = %d, want %d (body=%q)", userInfoRec.Code, http.StatusOK, userInfoRec.Body.String())
	}
	var userInfo userInfoBody
	if err := json.Unmarshal(userInfoRec.Body.Bytes(), &userInfo); err != nil {
		t.Fatalf("decode userinfo response: %v (body=%q)", err, userInfoRec.Body.String())
	}
	if userInfo.Subject != testUserID {
		t.Errorf("userinfo sub = %q, want %q", userInfo.Subject, testUserID)
	}
	if userInfo.Name != testUserName {
		t.Errorf("userinfo name = %q, want %q (profile scope was granted)", userInfo.Name, testUserName)
	}
	if userInfo.Email != testUserEmail {
		t.Errorf("userinfo email = %q, want %q (email scope was granted)", userInfo.Email, testUserEmail)
	}
}

// TestToken_PKCEMismatch_InvalidGrant covers traceability #3: a
// code_verifier that does not satisfy the bound code_challenge must
// fail the token exchange with invalid_grant.
func TestToken_PKCEMismatch_InvalidGrant(t *testing.T) {
	h := newTestHandler(t)

	verifierA := strings.Repeat("A", 43)
	verifierB := strings.Repeat("B", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifierA), "openid", "")

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifierB},
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := decodeErrorBody(t, rec).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
}

// TestToken_CodeReuse_InvalidGrant covers traceability #2: an
// authorization code, once redeemed, must not be redeemable again.
func TestToken_CodeReuse_InvalidGrant(t *testing.T) {
	h := newTestHandler(t)

	verifier := strings.Repeat("A", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifier), "openid", "")
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifier},
	}

	first := doToken(t, h, form)
	if first.Code != http.StatusOK {
		t.Fatalf("setup: first token exchange status = %d, want %d (body=%q)", first.Code, http.StatusOK, first.Body.String())
	}

	second := doToken(t, h, form)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", second.Code, http.StatusBadRequest, second.Body.String())
	}
	if got := decodeErrorBody(t, second).Error; got != "invalid_grant" {
		t.Errorf("error = %q, want %q", got, "invalid_grant")
	}
}

// TestAuthorize_UnknownClient_InvalidClient_DirectError covers
// traceability #1: an unverifiable client_id MUST be reported
// directly (never via redirect, to avoid an open-redirect).
func TestAuthorize_UnknownClient_InvalidClient_DirectError(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	rec := doAuthorize(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {"unknown-client"},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	})

	if rec.Code == http.StatusFound {
		t.Fatalf("status = %d (redirect), want a direct non-redirect error response", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("Location header = %q, want empty (unknown client must not be redirected)", loc)
	}
	if got := decodeErrorBody(t, rec).Error; got != "invalid_client" {
		t.Errorf("error = %q, want %q", got, "invalid_client")
	}
}

// TestAuthorize_MissingOpenIDScope_InvalidScope covers traceability
// #1: scope must include "openid". Since client_id/redirect_uri are
// valid here, the error is reported via redirect (not directly).
func TestAuthorize_MissingOpenIDScope_InvalidScope(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	rec := doAuthorize(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"profile"}, // no openid
		"state":                 {"s1"},
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
	if got := loc.Query().Get("error"); got != "invalid_scope" {
		t.Errorf("redirect error = %q, want %q", got, "invalid_scope")
	}
	if got := loc.Query().Get("state"); got != "s1" {
		t.Errorf("redirect state = %q, want %q", got, "s1")
	}
}

// TestAuthorize_ScopeNotAllowedForClient_InvalidScope covers
// traceability #1 from a different angle than the missing-openid
// case: openid is present, but an additional requested scope is not
// among the seeded client's allowed scopes ("openid"/"profile"/
// "email"; see newTestHandler), which authorization_service.Authorize
// rejects via client.Client.AllowsScope.
func TestAuthorize_ScopeNotAllowedForClient_InvalidScope(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	rec := doAuthorize(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid admin"}, // "admin" is not among the seeded client's allowed scopes
		"state":                 {"s3"},
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
	if got := loc.Query().Get("error"); got != "invalid_scope" {
		t.Errorf("redirect error = %q, want %q", got, "invalid_scope")
	}
	if got := loc.Query().Get("state"); got != "s3" {
		t.Errorf("redirect state = %q, want %q", got, "s3")
	}
}

// TestAuthorize_PlainChallengeMethod_InvalidRequest covers
// traceability #3: this authorization server accepts S256 only;
// code_challenge_method=plain must be rejected as invalid_request.
func TestAuthorize_PlainChallengeMethod_InvalidRequest(t *testing.T) {
	h := newTestHandler(t)

	rec := doAuthorize(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"s2"},
		"code_challenge":        {"some-arbitrary-challenge-value"},
		"code_challenge_method": {"plain"},
	})

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location header: %v", err)
	}
	if got := loc.Query().Get("error"); got != "invalid_request" {
		t.Errorf("redirect error = %q, want %q", got, "invalid_request")
	}
}

// TestUserInfo_InvalidBearer_Unauthorized covers traceability #7:
// missing or invalid bearer tokens must yield 401.
func TestUserInfo_InvalidBearer_Unauthorized(t *testing.T) {
	h := newTestHandler(t)

	t.Run("missing Authorization header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/userinfo", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
	})

	t.Run("malformed bearer token", func(t *testing.T) {
		rec := doUserInfo(t, h, "not-a-real-jwt")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
		if got := rec.Header().Get("WWW-Authenticate"); got == "" {
			t.Error("WWW-Authenticate header is empty, want set per RFC 6750")
		}
	})

	t.Run("Authorization header without Bearer prefix", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/userinfo", nil)
		req.Header.Set("Authorization", "Basic abc123")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
	})
}
