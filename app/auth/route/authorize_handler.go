package route

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// authorizeHandler serves GET /authorize (RFC 6749 4.1.1, OIDC Core
// 3.1.2.1).
type authorizeHandler struct {
	svc *service.AuthorizationService
}

// handle parses the authorization request's query parameters,
// delegates to AuthorizationService.Authorize, and redirects the
// user-agent back to the client's redirect_uri with the issued
// authorization code (RFC 6749 4.1.2), or with an OAuth error (see
// response.go's writeAuthorizeError for the open-redirect-safe error
// handling contract).
//
// *** This authorization server does not implement a login/consent
// UI. A real implementation would, at this point, redirect an
// unauthenticated user-agent to a login page, then a consent page,
// and only call AuthorizationService.Authorize once the resource
// owner has authenticated and approved the requested scope. This
// sample instead resolves the resource owner automatically inside
// AuthorizationService.Authorize (see resolveOwner there); this is
// the exact spot in the request flow where that real UI would be
// inserted. ***
func (h *authorizeHandler) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	req := service.AuthorizeRequest{
		ResponseType:        q.Get("response_type"),
		ClientID:            q.Get("client_id"),
		RedirectURI:         q.Get("redirect_uri"),
		Scope:               q.Get("scope"),
		State:               q.Get("state"),
		Nonce:               q.Get("nonce"),
		CodeChallenge:       q.Get("code_challenge"),
		CodeChallengeMethod: q.Get("code_challenge_method"),
		LoginHint:           q.Get("login_hint"),
	}

	result, err := h.svc.Authorize(r.Context(), req)
	if err != nil {
		// req.RedirectURI/req.State are the *raw, unvalidated* request
		// values; writeAuthorizeError only ever redirects to
		// req.RedirectURI once AuthorizationService.Authorize itself
		// has confirmed it is a registered redirect_uri for a known
		// client (see that function's ordering contract).
		writeAuthorizeError(w, r, req.RedirectURI, req.State, err)
		return
	}

	u, err := url.Parse(result.RedirectURI)
	if err != nil {
		// Should not happen: result.RedirectURI was already validated
		// by the service layer.
		slog.Error("route: authorize: parse validated redirect uri", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}
	q2 := u.Query()
	q2.Set("code", result.Code)
	if result.State != "" {
		q2.Set("state", result.State)
	}
	u.RawQuery = q2.Encode()

	http.Redirect(w, r, u.String(), http.StatusFound)
}
