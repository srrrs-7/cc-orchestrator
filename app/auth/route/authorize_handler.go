package route

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

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

// parseMaxAge returns the max_age parameter in seconds and whether it was
// present. The boolean is false when max_age is absent or malformed. Negative
// values are treated as malformed (absent).
func parseMaxAge(r *http.Request) (maxAge int64, set bool) {
	s := r.URL.Query().Get("max_age")
	if s == "" {
		return 0, false
	}
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// sanitizedPendingQuery returns the /authorize query without prompt
// and max_age so the restored authorize request after login does not
// loop (OIDC Core 3.1.2.1: prompt=login is a one-shot signal).
func sanitizedPendingQuery(r *http.Request) string {
	q := r.URL.Query()
	q.Del("prompt")
	q.Del("max_age")
	return q.Encode()
}

// writeAuthorizeErrorRedirect redirects to redirectURI with the given
// OAuth/OIDC error code per OIDC Core 3.1.2.6 (login_required,
// consent_required, etc.). Used when prompt=none forbids interactive UI.
func writeAuthorizeErrorRedirect(w http.ResponseWriter, r *http.Request, redirectURI, state, oauthErrorCode string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		slog.Error("route: authorize: parse redirect uri for authorize error", "error", err, "error_code", oauthErrorCode)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}
	q := u.Query()
	q.Set("error", oauthErrorCode)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// writeLoginRequiredRedirect redirects to redirectURI with
// error=login_required per OIDC Core 3.1.2.6, used when prompt=none
// but the resource owner has no active session.
func writeLoginRequiredRedirect(w http.ResponseWriter, r *http.Request, redirectURI, state string) {
	writeAuthorizeErrorRedirect(w, r, redirectURI, state, "login_required")
}

// handle parses the authorization request's query parameters,
// enforces prompt / max_age semantics (OIDC Core 3.1.2.1), ensures
// the resource owner is authenticated at the IdP (redirecting to
// /login when not), delegates to AuthorizationService.Authorize, and
// redirects the user-agent back to the client's redirect_uri.
func (h *authorizeHandler) handle(w http.ResponseWriter, r *http.Request) {
	req := parseAuthorizeRequest(r)
	prompt := r.URL.Query().Get("prompt")
	maxAge, maxAgeSet := parseMaxAge(r)

	verified, err := h.svc.ValidateAuthorize(r.Context(), req)
	if err != nil {
		writeAuthorizeError(w, r, verified.RedirectURI, req.State, err)
		return
	}

	// Attempt to find the IdP session. We use FindSession (not
	// UserFromSession) here so that prompt and max_age can be
	// evaluated against the session's AuthenticatedAt before the
	// user lookup.
	sessionID := readSessionCookie(r)
	sess, sessErr := h.authn.FindSession(r.Context(), sessionID)
	noSession := sessErr != nil

	// --- prompt=none (OIDC Core 3.1.2.1) ---
	// Must not show any interactive UI. If there is no session, return
	// login_required immediately via redirect (never redirect to /login).
	if prompt == "none" && noSession {
		writeLoginRequiredRedirect(w, r, verified.RedirectURI, req.State)
		return
	}

	// --- prompt=login (OIDC Core 3.1.2.1) ---
	// Force re-authentication even if a session already exists.
	if prompt == "login" && !noSession {
		if logoutErr := h.authn.Logout(r.Context(), sess.ID); logoutErr != nil {
			slog.Error("route: authorize: prompt=login: delete session", "error", logoutErr)
		}
		clearSessionCookie(w, h.secureCookies)
		// Strip prompt (and max_age) from the pending query so the
		// restored authorize request after login does not loop.
		pendingID, saveErr := h.authn.SavePendingAuthorize(r.Context(), sanitizedPendingQuery(r))
		if saveErr != nil {
			slog.Error("route: authorize: prompt=login: save pending", "error", saveErr)
			writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
			return
		}
		setPendingCookie(w, pendingID, h.secureCookies)
		http.Redirect(w, r, issuerPath(h.issuer, "/login"), http.StatusFound)
		return
	}

	// --- max_age (OIDC Core 3.1.2.1) ---
	// If the session is older than max_age seconds, force re-login.
	// max_age=0 means "must authenticate right now" (any elapsed time
	// is too old), so we check maxAgeSet to distinguish "not provided"
	// from an explicit max_age=0.
	if !noSession && maxAgeSet {
		if time.Since(sess.AuthenticatedAt) > time.Duration(maxAge)*time.Second {
			if logoutErr := h.authn.Logout(r.Context(), sess.ID); logoutErr != nil {
				slog.Error("route: authorize: max_age exceeded: delete session", "error", logoutErr)
			}
			clearSessionCookie(w, h.secureCookies)
			if prompt == "none" {
				writeLoginRequiredRedirect(w, r, verified.RedirectURI, req.State)
				return
			}
			// Treat as unauthenticated: fall through to the redirect-to-login path.
			noSession = true
		}
	}

	// --- Unauthenticated: redirect to /login ---
	if noSession {
		if errors.Is(sessErr, idpsession.ErrNotFound) || noSession {
			pendingID, saveErr := h.authn.SavePendingAuthorize(r.Context(), sanitizedPendingQuery(r))
			if saveErr != nil {
				slog.Error("route: authorize: save pending", "error", saveErr)
				writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
				return
			}
			setPendingCookie(w, pendingID, h.secureCookies)
			http.Redirect(w, r, issuerPath(h.issuer, "/login"), http.StatusFound)
			return
		}
		slog.Error("route: authorize: session lookup", "error", sessErr)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}

	// Session is valid. Resolve the resource owner.
	owner, err := h.authn.UserFromSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, idpsession.ErrNotFound) {
			// Session vanished between FindSession and UserFromSession
			// (e.g. concurrent logout); treat as unauthenticated.
			pendingID, saveErr := h.authn.SavePendingAuthorize(r.Context(), sanitizedPendingQuery(r))
			if saveErr != nil {
				slog.Error("route: authorize: save pending (session race)", "error", saveErr)
				writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
				return
			}
			setPendingCookie(w, pendingID, h.secureCookies)
			http.Redirect(w, r, issuerPath(h.issuer, "/login"), http.StatusFound)
			return
		}
		slog.Error("route: authorize: user from session", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}

	// Carry the session's AuthenticatedAt into the AuthorizeRequest so
	// the application layer can store it on the authorization code and
	// propagate it to ID token auth_time claims (OIDC Core §2).
	req.AuthTime = sess.AuthenticatedAt

	hasConsent, err := h.consent.HasGrant(r.Context(), owner.ID(), req.ClientID, req.Scope)
	if err != nil {
		slog.Error("route: authorize: consent lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}
	if !hasConsent {
		// prompt=none must not show the consent UI (OIDC Core 3.1.2.6).
		if prompt == "none" {
			writeAuthorizeErrorRedirect(w, r, verified.RedirectURI, req.State, "consent_required")
			return
		}
		pendingID, saveErr := h.authn.SavePendingAuthorize(r.Context(), sanitizedPendingQuery(r))
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
