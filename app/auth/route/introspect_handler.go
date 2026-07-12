package route

import (
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// introspectHandler serves POST /introspect (RFC 7662).
// For any token whose validity cannot be confirmed (invalid, expired,
// malformed, wrong audience) it returns {"active":false} per RFC 7662
// §2.2 -- never an HTTP error response.
type introspectHandler struct {
	svc     *service.IntrospectionService
	clients client.Repository
}

func (h *introspectHandler) handle(w http.ResponseWriter, r *http.Request) {
	if !parseFormBody(w, r) {
		return
	}

	clientID, clientSecret := extractClientCredentials(r)
	if err := authenticateClient(r.Context(), h.clients, clientID, clientSecret); err != nil {
		writeClientAuthError(w, err)
		return
	}

	tokenString := r.PostFormValue("token")
	resp := h.svc.Introspect(r.Context(), tokenString)
	writeJSON(w, http.StatusOK, resp)
}
