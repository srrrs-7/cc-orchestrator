package route

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	idpSessionCookieName   = "idp_session"
	idpPendingCookieName   = "idp_pending"
	idpSessionCookieMaxAge = 24 * 60 * 60
	idpPendingCookieMaxAge = 10 * 60
)

var pendingAuthorizeQueryKeys = map[string]struct{}{
	"response_type":         {},
	"client_id":             {},
	"redirect_uri":          {},
	"scope":                 {},
	"state":                 {},
	"nonce":                 {},
	"code_challenge":        {},
	"code_challenge_method": {},
	"login_hint":            {},
}

func readSessionCookie(r *http.Request) string {
	c, err := r.Cookie(idpSessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func readPendingCookie(r *http.Request) string {
	c, err := r.Cookie(idpPendingCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func setSessionCookie(w http.ResponseWriter, sessionID string, secure bool) {
	setIDPCookie(w, idpSessionCookieName, sessionID, idpSessionCookieMaxAge, secure)
}

func setPendingCookie(w http.ResponseWriter, pendingID string, secure bool) {
	setIDPCookie(w, idpPendingCookieName, pendingID, idpPendingCookieMaxAge, secure)
}

func clearPendingCookie(w http.ResponseWriter, secure bool) {
	//nolint:gosec // G124: Secure follows ISSUER scheme (false for local http compose); HttpOnly and SameSite=Lax are always set.
	http.SetCookie(w, &http.Cookie{
		Name:     idpPendingCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Unix(0, 0),
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	//nolint:gosec // G124: Secure follows ISSUER scheme (false for local http compose); HttpOnly and SameSite=Lax are always set.
	http.SetCookie(w, &http.Cookie{
		Name:     idpSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Unix(0, 0),
	})
}

func setIDPCookie(w http.ResponseWriter, name, value string, maxAge int, secure bool) {
	//nolint:gosec // G124: Secure follows ISSUER scheme (false for local http compose); HttpOnly and SameSite=Lax are always set.
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

// pendingAuthorizeLocation rebuilds the issuer-scoped /authorize URL with
// a pending query captured server-side during an earlier validated request.
func pendingAuthorizeLocation(issuer, rawQuery string) (string, bool) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", false
	}
	for key := range values {
		if _, ok := pendingAuthorizeQueryKeys[key]; !ok {
			return "", false
		}
	}
	return issuerPath(issuer, "/authorize") + "?" + rawQuery, true
}

func issuerPath(issuer, path string) string {
	return strings.TrimSuffix(issuer, "/") + path
}
