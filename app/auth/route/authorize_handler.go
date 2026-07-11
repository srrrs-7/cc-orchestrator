package route

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/idpsession"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// authorizeHandler serves GET /authorize (RFC 6749 4.1.1, OIDC Core
// 3.1.2.1).
type authorizeHandler struct {
	svc           *service.AuthorizationService
	authn         *service.AuthenticationService
	consent       *service.ConsentService
	issuer        string
	secureCookies bool
}

func parseAuthorizeRequest(r *http.Request) service.AuthorizeRequest {
	q := r.URL.Query()
	return service.AuthorizeRequest{
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
}

// handle parses the authorization request's query parameters,
// ensures the resource owner is authenticated at the IdP (redirecting
// to /login when not), delegates to AuthorizationService.Authorize,
// and redirects the user-agent back to the client's redirect_uri.
func (h *authorizeHandler) handle(w http.ResponseWriter, r *http.Request) {
	req := parseAuthorizeRequest(r)

	verified, err := h.svc.ValidateAuthorize(r.Context(), req)
	if err != nil {
		writeAuthorizeError(w, r, verified.RedirectURI, req.State, err)
		return
	}

	sessionID := readSessionCookie(r)
	owner, err := h.authn.UserFromSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, idpsession.ErrNotFound) {
			pendingID, saveErr := h.authn.SavePendingAuthorize(r.Context(), r.URL.RawQuery)
			if saveErr != nil {
				slog.Error("route: authorize: save pending", "error", saveErr)
				writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
				return
			}
			setPendingCookie(w, pendingID, h.secureCookies)
			http.Redirect(w, r, issuerPath(h.issuer, "/login"), http.StatusFound)
			return
		}
		slog.Error("route: authorize: session lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}

	// Carry the session's AuthenticatedAt into the AuthorizeRequest so
	// the application layer can store it on the authorization code and
	// propagate it to ID token auth_time claims (OIDC Core §2).
	if sess, sessErr := h.authn.FindSession(r.Context(), sessionID); sessErr == nil {
		req.AuthTime = sess.AuthenticatedAt
	}

	hasConsent, err := h.consent.HasGrant(r.Context(), owner.ID(), req.ClientID, req.Scope)
	if err != nil {
		slog.Error("route: authorize: consent lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}
	if !hasConsent {
		pendingID, saveErr := h.authn.SavePendingAuthorize(r.Context(), r.URL.RawQuery)
		if saveErr != nil {
			slog.Error("route: authorize: save pending consent", "error", saveErr)
			writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
			return
		}
		setPendingCookie(w, pendingID, h.secureCookies)
		http.Redirect(w, r, issuerPath(h.issuer, "/consent"), http.StatusFound)
		return
	}

	result, err := h.svc.Authorize(r.Context(), req, owner)
	if err != nil {
		writeAuthorizeError(w, r, result.RedirectURI, req.State, err)
		return
	}

	u, err := url.Parse(result.RedirectURI)
	if err != nil {
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
