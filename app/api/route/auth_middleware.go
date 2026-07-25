package route

import (
	"context"
	"net/http"
	"strings"
)

// TokenVerifier is the interface the auth middleware requires.
// Defined here (in the consuming package) per DDD convention: keep
// interfaces minimal and at the point of use, not at the implementation.
type TokenVerifier interface {
	Verify(ctx context.Context, tokenString string) error
}

// AuthMiddleware wraps next with Bearer JWT authentication. It extracts
// the token from the Authorization header, validates it via v, and
// returns a 401 JSON error body (matching the existing errorResponse
// format) if the token is missing or invalid. Valid requests are
// forwarded to next unchanged.
//
// When v is nil the middleware is a no-op (all requests pass through).
// This lets cmd/api/main.go skip wiring the verifier when AUTH_ISSUER /
// AUTH_JWKS_URL are unset (development without an auth server).
func AuthMiddleware(v TokenVerifier, next http.Handler) http.Handler {
	if v == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(raw, "Bearer ")
		if !ok || token == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing or invalid Authorization header"})
			return
		}
		if err := v.Verify(r.Context(), token); err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid or expired token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
