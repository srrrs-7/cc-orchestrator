// Regression tests for ISSUE-042 (app/auth had no clickjacking / MIME-
// sniffing defense of its own, relying entirely on nginx/CloudFront
// headers that the production /auth/* path bypasses). These assert
// that the securityHeaders middleware (route/security_headers.go)
// applies to representative handlers across the router: the
// server-rendered login/consent HTML (the endpoints the issue is
// specifically about, both the 200 render and a redirect response
// that never reaches the template), and JSON API endpoints (/token,
// /.well-known/openid-configuration, /.well-known/jwks.json).
//
// TestSecurityHeaders_Login, _ConsentRedirect and _JWKS use
// newDiscoveryTestHandler / newDiscoveryTestHandler-like nil-repo
// wiring: none of those exercised paths reach a repository (see
// helpers_test.go's package doc), so no DB is required.
// TestSecurityHeaders_ConsentRender and _TokenSuccess drive a full
// login -> authorize -> consent/token flow via newTestHandler, which
// does require a real Postgres test DB (again, see helpers_test.go's
// package doc) because /token and an actually-granted /consent GET do
// reach the client/user/authcode repositories.
package route_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// wantSecurityHeaders asserts the full set of headers securityHeaders
// sets, and in particular the two that directly fix ISSUE-042's
// clickjacking vector: X-Frame-Options: DENY and a CSP whose
// frame-ancestors directive is 'none'.
func wantSecurityHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want %q", got, "DENY")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	if !containsDirective(csp, "frame-ancestors 'none'") {
		t.Errorf("Content-Security-Policy = %q, want it to contain %q", csp, "frame-ancestors 'none'")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := rec.Header().Get("Referrer-Policy"); got == "" {
		t.Error("Referrer-Policy header is missing")
	}
}

func containsDirective(csp, directive string) bool {
	// A plain substring check is sufficient here: securityHeadersCSP is
	// a fixed constant this test also indirectly pins, and CSP
	// directives are ';'-separated tokens that never legitimately
	// overlap as substrings of one another for the values under test.
	for i := 0; i+len(directive) <= len(csp); i++ {
		if csp[i:i+len(directive)] == directive {
			return true
		}
	}
	return false
}

// TestSecurityHeaders_Login covers GET /login, the server-rendered
// sign-in form ISSUE-042 identifies as clickjacking-vulnerable.
func TestSecurityHeaders_Login(t *testing.T) {
	h := newDiscoveryTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantSecurityHeaders(t, rec)
}

// TestSecurityHeaders_ConsentRedirect covers GET /consent without an
// IdP session cookie. It never reaches the consent HTML template (it
// 302s to /login instead), which is exactly why it is a useful case:
// it proves the headers are set by middleware wrapping every response,
// not by the template-rendering code path alone.
func TestSecurityHeaders_ConsentRedirect(t *testing.T) {
	h := newDiscoveryTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/consent", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	wantSecurityHeaders(t, rec)
}

// TestSecurityHeaders_JWKS covers a JSON API endpoint
// (/.well-known/jwks.json) to confirm the middleware applies uniformly
// across the router rather than being special-cased to HTML handlers.
func TestSecurityHeaders_JWKS(t *testing.T) {
	h := newDiscoveryTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantSecurityHeaders(t, rec)
}

// TestSecurityHeaders_DiscoveryMetadata covers the other JSON discovery
// endpoint, /.well-known/openid-configuration, alongside JWKS above.
func TestSecurityHeaders_DiscoveryMetadata(t *testing.T) {
	h := newDiscoveryTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantSecurityHeaders(t, rec)
}

// TestSecurityHeaders_ConsentRender covers GET /consent with a valid
// session and pending-authorize cookie, i.e. the actual 200 render of
// consent.html -- as opposed to TestSecurityHeaders_ConsentRedirect
// above, which 302s before ever reaching the template. Together the
// two prove the middleware applies regardless of which code path
// inside consentHandler.handleGet runs.
func TestSecurityHeaders_ConsentRender(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)
	session := loginSession(t, h)

	authRec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid profile email"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}, session)
	if got := authRec.Header().Get("Location"); got != testIssuer+"/consent" {
		t.Fatalf("setup: expected consent redirect, got %q", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/consent", nil)
	req.AddCookie(session)
	if pending := cookieFromResponse(authRec, "idp_pending"); pending != nil {
		req.AddCookie(pending)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantSecurityHeaders(t, rec)
}

// TestSecurityHeaders_TokenSuccess covers a successful POST /token
// (grant_type=authorization_code) response: the JSON API endpoint the
// issue's own "existing JSON response tests still pass" concern is
// most directly about. It also confirms the new middleware coexists
// with /token's pre-existing no-store cache headers (security_test.go
// traceability #4 / R10) rather than clobbering them -- securityHeaders
// only ever Set()s its own four header keys.
func TestSecurityHeaders_TokenSuccess(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifier), "openid profile email", "")

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifier},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantSecurityHeaders(t, rec)

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q (securityHeaders must not clobber existing headers)", got, "no-store")
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Errorf("Pragma = %q, want %q (securityHeaders must not clobber existing headers)", got, "no-cache")
	}
}
