package route

import (
	"net/http"

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
func (h *tokenHandler) handle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		setTokenNoCacheHeaders(w)
		writeBadRequest(w, "malformed form body")
		return
	}

	req := service.TokenRequest{
		GrantType:    r.PostFormValue("grant_type"),
		Code:         r.PostFormValue("code"),
		RedirectURI:  r.PostFormValue("redirect_uri"),
		ClientID:     r.PostFormValue("client_id"),
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
