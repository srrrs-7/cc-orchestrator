package route

import (
	"context"
	"net/http"
	"net/url"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

type logoutHandler struct {
	authn         *service.AuthenticationService
	clients       client.Repository
	issuer        string
	secureCookies bool
}

func newLogoutHandler(
	authn *service.AuthenticationService,
	clients client.Repository,
	issuer string,
	secureCookies bool,
) *logoutHandler {
	return &logoutHandler{authn: authn, clients: clients, issuer: issuer, secureCookies: secureCookies}
}

func (h *logoutHandler) handle(w http.ResponseWriter, r *http.Request) {
	if err := h.authn.Logout(r.Context(), readSessionCookie(r)); err != nil {
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}
	clearSessionCookie(w, h.secureCookies)

	postLogout := r.URL.Query().Get("post_logout_redirect_uri")
	state := r.URL.Query().Get("state")
	clientID := r.URL.Query().Get("client_id")

	if postLogout != "" && clientID != "" && h.isAllowedPostLogoutRedirect(r.Context(), clientID, postLogout) {
		target, err := url.Parse(postLogout)
		if err != nil {
			http.Redirect(w, r, issuerPath(h.issuer, "/login?logged_out=1"), http.StatusFound)
			return
		}
		if state != "" {
			q := target.Query()
			q.Set("state", state)
			target.RawQuery = q.Encode()
		}
		//nolint:gosec // G704: post_logout_redirect_uri was validated against registered client redirect URIs above.
		http.Redirect(w, r, target.String(), http.StatusFound)
		return
	}

	http.Redirect(w, r, issuerPath(h.issuer, "/login?logged_out=1"), http.StatusFound)
}

func (h *logoutHandler) isAllowedPostLogoutRedirect(ctx context.Context, clientID, postLogout string) bool {
	cid, err := client.ParseClientID(clientID)
	if err != nil {
		return false
	}
	c, err := h.clients.FindByID(ctx, cid)
	if err != nil {
		return false
	}
	redirectURI, err := client.NewRedirectURI(postLogout)
	if err != nil {
		return false
	}
	return c.ValidateRedirectURI(redirectURI) == nil
}
