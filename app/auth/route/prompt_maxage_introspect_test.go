package route_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// prompt=login tests
// ---------------------------------------------------------------------------

// TestAuthorize_PromptLogin_WithSession_ForcesReauth verifies that when an
// active session exists and prompt=login is requested, the server clears the
// session and redirects to /login (OIDC Core 3.1.2.1).
func TestAuthorize_PromptLogin_WithSession_ForcesReauth(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	// First: establish a session by doing a full authorize flow.
	_ = issueAuthCode(t, h, pkceChallenge(verifier), "openid", "")

	// Second: use the session cookie with prompt=login on a fresh authorize.
	// The handler should delete the session and redirect to /login.
	session := loginSession(t, h)
	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"s1"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"prompt":                {"login"},
	}, session)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasSuffix(loc, "/login") {
		t.Errorf("Location = %q, want to end with /login", loc)
	}
}

// TestAuthorize_PromptLogin_NoSession_RedirectsToLogin verifies that when no
// session exists and prompt=login is requested, the server redirects to /login
// as it would for any unauthenticated request.
func TestAuthorize_PromptLogin_NoSession_RedirectsToLogin(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	// No session cookie provided.
	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"s2"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"prompt":                {"login"},
	}, nil)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasSuffix(loc, "/login") {
		t.Errorf("Location = %q, want to end with /login", loc)
	}
}

// TestAuthorize_PromptLogin_PendingQueryStripsPrompt verifies that after
// prompt=login clears the session and saves a pending authorize, the pending
// query does NOT contain "prompt=login" (which would cause an infinite loop
// after the user logs in again).
func TestAuthorize_PromptLogin_PendingQueryStripsPrompt(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	// Establish a session.
	session := loginSession(t, h)

	// Make an authorize request with prompt=login.
	authorizeRec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"loop-check"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"prompt":                {"login"},
	}, session)

	if authorizeRec.Code != http.StatusFound {
		t.Fatalf("authorize (prompt=login) status = %d, want %d", authorizeRec.Code, http.StatusFound)
	}
	// The pending cookie should be set (pointing to the saved query).
	pending := cookieFromResponse(authorizeRec, "idp_pending")
	if pending == nil {
		t.Fatal("idp_pending cookie not set after prompt=login redirect")
	}

	// Log in again. After login, the handler reconstructs the authorize URL
	// from the pending store. It must NOT contain prompt=login.
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{
		"username": {testUsername},
		"password": {testDemoPassword},
	}.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.AddCookie(pending)
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want %d", loginRec.Code, http.StatusFound)
	}
	loc := loginRec.Header().Get("Location")
	if strings.Contains(loc, "prompt=login") {
		t.Errorf("Location after login = %q; must not contain prompt=login (would cause infinite loop)", loc)
	}
}

// ---------------------------------------------------------------------------
// prompt=none tests
// ---------------------------------------------------------------------------

// TestAuthorize_PromptNone_NoSession_LoginRequired verifies that when
// prompt=none is requested and no session exists, the server redirects the
// user-agent back to the client with error=login_required (OIDC Core 3.1.2.6)
// rather than redirecting to /login.
func TestAuthorize_PromptNone_NoSession_LoginRequired(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	// No session cookie.
	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"none-state"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"prompt":                {"none"},
	}, nil)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	// Must redirect back to the client (redirect_uri host), not to /login.
	if loc.Host == "" || strings.HasSuffix(loc.Path, "/login") {
		t.Errorf("Location = %q, must redirect to client redirect_uri (not /login)", loc.String())
	}
	if got := loc.Query().Get("error"); got != "login_required" {
		t.Errorf("error = %q, want %q", got, "login_required")
	}
	if got := loc.Query().Get("state"); got != "none-state" {
		t.Errorf("state = %q, want %q", got, "none-state")
	}
}

// TestAuthorize_PromptNone_WithSession_Succeeds verifies that when prompt=none
// is requested and a valid session already exists (with prior consent), the
// server issues the authorization code normally.
func TestAuthorize_PromptNone_WithSession_Succeeds(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	// Establish a session AND grant consent by completing a full authorize flow.
	_ = issueAuthCode(t, h, pkceChallenge(verifier), "openid", "")

	// Now use the same session with prompt=none.
	session := loginSession(t, h)

	// First authorize to grant consent.
	firstRec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"pre-none"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}, session)
	if firstRec.Code == http.StatusFound {
		loc := firstRec.Header().Get("Location")
		if strings.HasSuffix(loc, "/consent") {
			acceptConsent(t, h, session, firstRec)
		}
	}

	// Now request with prompt=none; consent should already be granted.
	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"none-ok"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"prompt":                {"none"},
	}, session)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := loc.Query().Get("error"); got != "" {
		t.Errorf("unexpected error = %q, want empty (should succeed with active session+consent)", got)
	}
	if code := loc.Query().Get("code"); code == "" {
		t.Error("authorization code is empty, want non-empty")
	}
}

// ---------------------------------------------------------------------------
// max_age tests
// ---------------------------------------------------------------------------

// TestAuthorize_MaxAge_FreshSession_Succeeds verifies that when max_age is
// large enough that the session age is within it, the flow proceeds normally.
func TestAuthorize_MaxAge_FreshSession_Succeeds(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	// Establish a session and consent via a full flow.
	_ = issueAuthCode(t, h, pkceChallenge(verifier), "openid", "")

	session := loginSession(t, h)
	// Accept consent first.
	firstRec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}, session)
	if strings.HasSuffix(firstRec.Header().Get("Location"), "/consent") {
		acceptConsent(t, h, session, firstRec)
	}

	// max_age=3600 (1 hour): a freshly created session should pass.
	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"ma-ok"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"max_age":               {"3600"},
	}, session)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := loc.Query().Get("error"); got != "" {
		t.Errorf("unexpected error = %q, want empty (fresh session within max_age)", got)
	}
	if code := loc.Query().Get("code"); code == "" {
		t.Error("authorization code is empty, want non-empty (fresh session should succeed)")
	}
}

// TestAuthorize_MaxAge_Zero_RedirectsToLogin verifies that max_age=0 forces
// re-authentication (zero age means "must have just authenticated").
func TestAuthorize_MaxAge_Zero_RedirectsToLogin(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)

	// Establish a session (even a fresh one is older than 0 seconds
	// due to wall-clock elapsed time).
	session := loginSession(t, h)

	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"ma-zero"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"max_age":               {"0"},
	}, session)

	// max_age=0 means the session age (even 1ms) > 0s, so re-auth is forced.
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasSuffix(loc, "/login") {
		t.Errorf("Location = %q, want to end with /login (max_age=0 forces re-auth)", loc)
	}
}

// ---------------------------------------------------------------------------
// POST /introspect tests
// ---------------------------------------------------------------------------

// doIntrospect sends a POST /introspect request with the given token.
// clientID (and clientSecret when needed) authenticate the caller per RFC 7662 §2.1.
func doIntrospect(t *testing.T, h http.Handler, token, clientID, clientSecret string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"token": {token}}
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req := httptest.NewRequest(http.MethodPost, "/introspect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type introspectionBody struct {
	Active  bool   `json:"active"`
	Subject string `json:"sub"`
	Exp     int64  `json:"exp"`
	Scope   string `json:"scope"`
}

func decodeIntrospectionBody(t *testing.T, rec *httptest.ResponseRecorder) introspectionBody {
	t.Helper()
	var got introspectionBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode introspection response: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

// TestIntrospect_ValidAccessToken_Active verifies that a freshly issued access
// token introspects as active=true with the expected claims (RFC 7662 §2.2).
func TestIntrospect_ValidAccessToken_Active(t *testing.T) {
	h := newTestHandler(t)

	// Issue an access token via the full authorize→token flow.
	tokens := issueTokens(t, h, "openid", "")
	accessToken := tokens.AccessToken
	if accessToken == "" {
		t.Fatal("setup: access_token is empty")
	}

	rec := doIntrospect(t, h, accessToken, testClientID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := decodeIntrospectionBody(t, rec)
	if !body.Active {
		t.Error("active = false, want true for a valid access token")
	}
	if body.Subject != testUserID {
		t.Errorf("sub = %q, want %q", body.Subject, testUserID)
	}
	if body.Exp == 0 {
		t.Error("exp is zero, want non-zero")
	}
	if !strings.Contains(body.Scope, "openid") {
		t.Errorf("scope = %q, want to contain openid", body.Scope)
	}
}

// TestIntrospect_InvalidToken_Inactive verifies that a malformed/garbage
// token returns active=false (RFC 7662 §2.2: must never error).
func TestIntrospect_InvalidToken_Inactive(t *testing.T) {
	h := newTestHandler(t)

	rec := doIntrospect(t, h, "not.a.real.jwt", testClientID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeIntrospectionBody(t, rec)
	if body.Active {
		t.Error("active = true, want false for a garbage token")
	}
}

// TestIntrospect_EmptyToken_Inactive verifies that an empty token
// parameter returns active=false (RFC 7662 §2.2).
func TestIntrospect_EmptyToken_Inactive(t *testing.T) {
	h := newTestHandler(t)

	rec := doIntrospect(t, h, "", testClientID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeIntrospectionBody(t, rec)
	if body.Active {
		t.Error("active = true, want false for empty token")
	}
}

// TestIntrospect_IDToken_Inactive verifies that an ID token (aud=client_id,
// not the API audience) introspects as inactive. The introspection endpoint
// only validates access tokens (aud=apiAudience).
func TestIntrospect_IDToken_Inactive(t *testing.T) {
	h := newTestHandler(t)

	tokens := issueTokens(t, h, "openid", "nonce-val")
	idToken := tokens.IDToken
	if idToken == "" {
		t.Fatal("setup: id_token is empty")
	}

	rec := doIntrospect(t, h, idToken, testClientID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeIntrospectionBody(t, rec)
	if body.Active {
		t.Error("active = true for an ID token, want false (aud mismatch: ID token aud=client_id, not apiAudience)")
	}
}

// TestIntrospect_NoClientAuth_Unauthorized verifies RFC 7662 §2.1: the
// introspection endpoint rejects unauthenticated callers.
func TestIntrospect_NoClientAuth_Unauthorized(t *testing.T) {
	h := newTestHandler(t)

	rec := doIntrospect(t, h, "any-token", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	got := decodeErrorBody(t, rec)
	if got.Error != "invalid_client" {
		t.Errorf("error = %q, want %q", got.Error, "invalid_client")
	}
}

// TestIntrospect_DiscoveryListsEndpoint verifies that the discovery metadata
// advertises the introspection_endpoint (optional per OIDC Discovery 1.0).
func TestIntrospect_DiscoveryListsEndpoint(t *testing.T) {
	h := newDiscoveryTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("discovery status = %d, want %d", rec.Code, http.StatusOK)
	}

	var meta map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	ep, ok := meta["introspection_endpoint"]
	if !ok {
		t.Error("introspection_endpoint absent from discovery metadata")
	}
	epStr, _ := ep.(string)
	if !strings.HasSuffix(epStr, "/introspect") {
		t.Errorf("introspection_endpoint = %q, want to end with /introspect", epStr)
	}
}
