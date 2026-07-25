package route_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorize_Unauthenticated_RedirectsToConsentAfterLogin(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)
	session := loginSession(t, h)

	rec := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid profile email"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}, session)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != testIssuer+"/consent" {
		t.Errorf("Location = %q, want %q", got, testIssuer+"/consent")
	}
}

func TestConsent_Deny_ReturnsAccessDenied(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)
	session := loginSession(t, h)
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"deny-state"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}

	authRec := doAuthorizeWithSession(t, h, query, session)
	if authRec.Header().Get("Location") != testIssuer+"/consent" {
		t.Fatalf("setup: expected consent redirect, got %q", authRec.Header().Get("Location"))
	}

	req := httptest.NewRequest(http.MethodPost, "/consent", strings.NewReader("action=deny"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)
	if pending := cookieFromResponse(authRec, "idp_pending"); pending != nil {
		req.AddCookie(pending)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := loc.Query().Get("error"); got != "access_denied" {
		t.Errorf("error = %q, want access_denied", got)
	}
	if got := loc.Query().Get("state"); got != "deny-state" {
		t.Errorf("state = %q, want deny-state", got)
	}
}

func TestAuthorize_SkipsConsentWhenAlreadyGranted(t *testing.T) {
	h := newTestHandler(t)
	verifier := strings.Repeat("A", 43)
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"state":                 {"s1"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}

	first := doAuthorize(t, h, query)
	if first.Header().Get("Location") == testIssuer+"/consent" {
		t.Fatalf("setup: first authorize should complete after consent accept, got consent redirect")
	}

	second := doAuthorize(t, h, query)
	if second.Code != http.StatusFound {
		t.Fatalf("second authorize status = %d, want %d", second.Code, http.StatusFound)
	}
	if strings.HasSuffix(second.Header().Get("Location"), "/consent") {
		t.Errorf("second authorize should skip consent, got %q", second.Header().Get("Location"))
	}
	loc, _ := url.Parse(second.Header().Get("Location"))
	if loc.Query().Get("code") == "" {
		t.Error("second authorize redirect missing code")
	}
}
