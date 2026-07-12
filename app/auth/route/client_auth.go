package route

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
)

// authenticateClient verifies that clientID identifies a registered client
// and, when the client is confidential, that clientSecret is correct (RFC
// 6749 §2.3.1). Public clients require client_id only.
func authenticateClient(ctx context.Context, clients client.Repository, clientID, clientSecret string) error {
	if clientID == "" {
		return client.ErrInvalidClientID
	}
	cid, err := client.ParseClientID(clientID)
	if err != nil {
		return err
	}
	c, err := clients.FindByID(ctx, cid)
	if err != nil {
		return err
	}
	if c.IsConfidential() && !c.VerifySecret(clientSecret) {
		return client.ErrClientAuthFailed
	}
	return nil
}

// writeClientAuthError maps client authentication failures to HTTP
// responses for endpoints that require authenticated callers (e.g.
// RFC 7662 introspection).
func writeClientAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, client.ErrInvalidClientID):
		writeJSON(w, http.StatusUnauthorized, oauthError{
			Error:            "invalid_client",
			ErrorDescription: "client_id is required",
		})
	case errors.Is(err, client.ErrNotFound):
		writeJSON(w, http.StatusUnauthorized, oauthError{
			Error:            "invalid_client",
			ErrorDescription: "unknown client",
		})
	case errors.Is(err, client.ErrClientAuthFailed):
		w.Header().Set("WWW-Authenticate", `Basic realm="oauth"`)
		writeJSON(w, http.StatusUnauthorized, oauthError{
			Error:            "invalid_client",
			ErrorDescription: "client authentication failed",
		})
	default:
		slog.Error("route: client auth: internal error", "error", err)
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
	}
}
