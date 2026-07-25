package route

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// tokenHandler serves POST /token (RFC 6749 4.1.3).
type tokenHandler struct {
	svc *service.AuthorizationService
}

// handle parses the token request's application/x-www-form-urlencoded
// body, delegates to AuthorizationService.Token, and returns the
// resulting access token + ID Token as JSON, or an OAuth error (RFC
// 6749 5.1/5.2). Every response (success or error) carries
// Cache-Control: no-store and Pragma: no-cache, since the body may
// contain a token (RFC 6749 5.1 MUST).
//
// Client authentication (ISSUE-035, RFC 6749 2.3):
//   - client_secret_basic: Authorization: Basic base64(client_id:client_secret)
//   - client_secret_post:  client_id + client_secret in the POST body
//   - none (public):       client_id in the POST body, no secret
func (h *tokenHandler) handle(w http.ResponseWriter, r *http.Request) {
	setTokenNoCacheHeaders(w)
	if !parseFormBody(w, r) {
		return
	}

	clientID, clientSecret := extractClientCredentials(r)

	req := service.TokenRequest{
		GrantType:    r.PostFormValue("grant_type"),
		Code:         r.PostFormValue("code"),
		RedirectURI:  r.PostFormValue("redirect_uri"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		CodeVerifier: r.PostFormValue("code_verifier"),
		RefreshToken: r.PostFormValue("refresh_token"),
		Scope:        r.PostFormValue("scope"),
	}

	resp, err := h.svc.Token(r.Context(), req)
	if err != nil {
		writeTokenError(w, err)
		return
	}

	setTokenNoCacheHeaders(w)
	writeJSON(w, http.StatusOK, resp)
}

// extractClientCredentials resolves the client_id and client_secret
// from the HTTP request, supporting both client_secret_basic
// (Authorization: Basic header, RFC 6749 2.3.1) and client_secret_post
// / none (form body fields). Basic auth takes precedence when both
// are present.
func extractClientCredentials(r *http.Request) (clientID, clientSecret string) {
	// client_secret_basic: Authorization: Basic base64url(client_id:client_secret)
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Basic ") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
		if err == nil {
			if idx := strings.IndexByte(string(decoded), ':'); idx >= 0 {
				return string(decoded[:idx]), string(decoded[idx+1:])
			}
		}
	}
	// client_secret_post / none: client credentials in the POST body
	return r.PostFormValue("client_id"), r.PostFormValue("client_secret")
}
