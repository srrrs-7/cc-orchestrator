// Package route is the presentation layer: it translates HTTP
// requests into application-layer (service) calls and translates
// their results (including domain errors) back into HTTP responses,
// following the OAuth 2.0 (RFC 6749) / OIDC error response contracts.
package route

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// oauthError is the JSON body returned for a failed /token or
// /userinfo request (RFC 6749 5.2).
type oauthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("route: encode json response", "error", err)
	}
}

// writeBadRequest is used for request-parsing failures (e.g.
// malformed form body) that never reach the application layer.
func writeBadRequest(w http.ResponseWriter, description string) {
	writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request", ErrorDescription: description})
}

// setTokenNoCacheHeaders sets the headers RFC 6749 5.1 requires on
// every /token response, success or error: both Cache-Control:
// no-store and Pragma: no-cache MUST be present (the latter is for
// HTTP/1.0 caches that do not understand Cache-Control).
func setTokenNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

// --- /authorize error handling ---
//
// RFC 6749 4.1.2.1 requires that if the client_id or redirect_uri
// cannot be verified, the authorization server MUST NOT redirect the
// user-agent and must instead inform the resource owner directly
// (otherwise this endpoint could be abused as an open redirector to
// an unregistered/attacker-controlled URI). AuthorizationService.Authorize
// is intentionally structured so that exactly the errors checked by
// isUnverifiedAuthorizeError occur before client_id/redirect_uri are
// confirmed; every other error is safe to report via redirect.

// isUnverifiedAuthorizeError reports whether err occurred before the
// client_id/redirect_uri were confirmed valid.
func isUnverifiedAuthorizeError(err error) bool {
	return errors.Is(err, client.ErrNotFound) ||
		errors.Is(err, client.ErrInvalidClientID) ||
		errors.Is(err, client.ErrInvalidRedirectURI) ||
		errors.Is(err, client.ErrRedirectURIMismatch)
}

// authorizeErrorCode maps a domain/service error from Authorize to
// its OAuth error code and a human-readable (non-sensitive)
// description. It returns ("", "") for errors that must not be
// exposed to the client (internal errors), which the caller logs via
// slog and reports as "server_error" instead.
func authorizeErrorCode(err error) (code, description string) {
	switch {
	case errors.Is(err, client.ErrNotFound):
		return "invalid_client", "unknown client"
	case errors.Is(err, client.ErrInvalidClientID):
		return "invalid_request", "client_id is required"
	case errors.Is(err, client.ErrInvalidRedirectURI):
		return "invalid_request", "redirect_uri is missing or malformed"
	case errors.Is(err, client.ErrRedirectURIMismatch):
		return "invalid_request", "redirect_uri does not match a registered redirect uri"
	case errors.Is(err, client.ErrUnsupportedResponseType):
		return "unsupported_response_type", "only response_type=code is supported"
	case errors.Is(err, authcode.ErrMissingOpenIDScope):
		return "invalid_scope", "scope must include openid"
	case errors.Is(err, authcode.ErrInvalidScope):
		return "invalid_scope", "scope is missing or not permitted for this client"
	case errors.Is(err, authcode.ErrUnsupportedChallengeMethod):
		return "invalid_request", "code_challenge_method must be S256"
	case errors.Is(err, authcode.ErrInvalidCodeVerifier):
		return "invalid_request", "code_challenge is missing or malformed"
	default:
		return "", ""
	}
}

// writeAuthorizeError renders an /authorize failure. When
// client_id/redirect_uri were already confirmed valid (i.e. err is
// not one of isUnverifiedAuthorizeError's cases, and redirectURI is
// non-empty), the error is reported by redirecting to redirectURI
// with ?error=...&error_description=...&state=... (RFC 6749
// 4.1.2.1). Otherwise it is rendered directly as a JSON body, never
// via redirect, to avoid this endpoint becoming an open redirector.
func writeAuthorizeError(w http.ResponseWriter, r *http.Request, redirectURI, state string, err error) {
	code, description := authorizeErrorCode(err)
	if code == "" {
		slog.Error("route: authorize: internal error", "error", err)
		code, description = "server_error", ""
	}

	if redirectURI == "" || isUnverifiedAuthorizeError(err) {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: code, ErrorDescription: description})
		return
	}

	u, parseErr := url.Parse(redirectURI)
	if parseErr != nil {
		// Should not happen: redirectURI was already validated by the
		// service layer by the time we reach here.
		slog.Error("route: authorize: parse validated redirect uri", "error", parseErr)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}

	q := u.Query()
	q.Set("error", code)
	if description != "" {
		q.Set("error_description", description)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusFound)
}

// --- /token error handling ---

// tokenErrorCode maps a domain/service error from Token to its HTTP
// status code and OAuth error code (RFC 6749 5.2). It returns
// (0, "", "") for errors that must not be exposed to the client, which
// the caller logs via slog and reports as a generic 500 instead.
func tokenErrorCode(err error) (status int, code, description string) {
	switch {
	case errors.Is(err, service.ErrUnsupportedGrantType), errors.Is(err, client.ErrUnsupportedGrantType):
		return http.StatusBadRequest, "unsupported_grant_type", "only grant_type=authorization_code and grant_type=refresh_token are supported"
	case errors.Is(err, client.ErrInvalidClientID):
		return http.StatusBadRequest, "invalid_request", "client_id is required"
	case errors.Is(err, client.ErrNotFound):
		return http.StatusBadRequest, "invalid_client", "unknown client"
	case errors.Is(err, user.ErrNotFound):
		// The authorization code / refresh token itself resolved
		// successfully but the resource owner it names no longer
		// exists (e.g. deleted after the grant was issued). RFC 6749
		// 5.2 has no dedicated code for this; invalid_grant is the
		// closest fit ("the ... authorization grant ... is invalid").
		// The description intentionally mirrors the other invalid_grant
		// cases below rather than mentioning the user, so this
		// response is indistinguishable from an ordinary expired/
		// already-used grant (ISSUE-019 課題 3: no user-existence
		// oracle via /token).
		return http.StatusBadRequest, "invalid_grant", "the authorization grant is invalid, expired, or already used"
	case errors.Is(err, authcode.ErrNotFound),
		errors.Is(err, authcode.ErrAlreadyConsumed),
		errors.Is(err, authcode.ErrExpired),
		errors.Is(err, authcode.ErrRedirectURIMismatch),
		errors.Is(err, authcode.ErrClientMismatch),
		errors.Is(err, authcode.ErrPKCEVerificationFailed),
		errors.Is(err, authcode.ErrInvalidCodeVerifier):
		return http.StatusBadRequest, "invalid_grant", "the authorization code is invalid, expired, already used, or code_verifier does not match"
	case errors.Is(err, refreshtoken.ErrInvalidScope):
		return http.StatusBadRequest, "invalid_scope", "requested scope exceeds the scope originally granted to this refresh token"
	case errors.Is(err, refreshtoken.ErrNotFound),
		errors.Is(err, refreshtoken.ErrReused),
		errors.Is(err, refreshtoken.ErrClientMismatch),
		errors.Is(err, refreshtoken.ErrExpired):
		return http.StatusBadRequest, "invalid_grant", "the refresh token is invalid, expired, already used, or was not issued to this client"
	default:
		return 0, "", ""
	}
}

// writeTokenError renders a /token failure as a JSON error body (RFC
// 6749 5.2), always with the same no-cache headers as a successful
// response (the body may still be sensitive to caching
// intermediaries).
func writeTokenError(w http.ResponseWriter, err error) {
	setTokenNoCacheHeaders(w)

	status, code, description := tokenErrorCode(err)
	if code == "" {
		slog.Error("route: token: internal error", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}
	writeJSON(w, status, oauthError{Error: code, ErrorDescription: description})
}

// --- /userinfo error handling ---

// writeUserInfoError renders a /userinfo failure. Per OIDC Core
// 5.3.3, an invalid/expired/malformed bearer token yields 401
// Unauthorized; any other (unexpected) error is logged and reported
// as a generic 500 without leaking internal details.
func writeUserInfoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, token.ErrInvalidToken),
		errors.Is(err, token.ErrTokenExpired),
		errors.Is(err, token.ErrSignatureInvalid),
		errors.Is(err, token.ErrUnexpectedAlg):
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeJSON(w, http.StatusUnauthorized, oauthError{Error: "invalid_token"})
	default:
		slog.Error("route: userinfo: internal error", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
	}
}
