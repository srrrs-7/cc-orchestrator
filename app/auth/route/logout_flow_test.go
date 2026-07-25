package route_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLogout_ClearsSessionAndRedirectsPostLogout(t *testing.T) {
	h := newTestHandler(t)
	session := loginSession(t, h)

	logoutURL := testIssuer + "/logout?" + url.Values{
		"client_id":                {testClientID},
		"post_logout_redirect_uri": {testRedirectURI},
		"state":                    {"logout-state"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, "/logout?"+strings.Split(logoutURL, "?")[1], nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if loc.String() != testRedirectURI+"?state=logout-state" && loc.Query().Get("state") != "logout-state" {
		t.Errorf("Location = %q, want redirect to post_logout_redirect_uri with state", rec.Header().Get("Location"))
	}

	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "idp_session" && (c.MaxAge < 0 || c.Value == "") {
			cleared = true
		}
	}
	if !cleared {
		t.Error("expected idp_session cookie to be cleared")
	}

	// Session must no longer authorize.
	rec2 := doAuthorizeWithSession(t, h, url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid"},
		"code_challenge":        {pkceChallenge(strings.Repeat("A", 43))},
		"code_challenge_method": {"S256"},
	}, session)
	if rec2.Header().Get("Location") != testIssuer+"/consent" && !strings.HasSuffix(rec2.Header().Get("Location"), "/consent") {
		// Without session, should redirect to login first; consent if session invalid means login redirect
		if !strings.HasSuffix(rec2.Header().Get("Location"), "/login") {
			t.Errorf("after logout, authorize with old session cookie should not succeed silently, Location=%q", rec2.Header().Get("Location"))
		}
	}
}

func TestLogout_WithoutPostLogoutRedirect_GoesToLogin(t *testing.T) {
	h := newTestHandler(t)
	session := loginSession(t, h)

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != testIssuer+"/login?logged_out=1" {
		t.Errorf("Location = %q, want %q", got, testIssuer+"/login?logged_out=1")
	}
}
