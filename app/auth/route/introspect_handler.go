package route

import (
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// introspectHandler serves POST /introspect (RFC 7662).
// For any token whose validity cannot be confirmed (invalid, expired,
// malformed, wrong audience) it returns {"active":false} per RFC 7662
// §2.2 -- never an HTTP error response.
type introspectHandler struct {
	svc *service.IntrospectionService
}

func (h *introspectHandler) handle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
		return
	}
	tokenString := r.PostFormValue("token")
	resp := h.svc.Introspect(r.Context(), tokenString)
	writeJSON(w, http.StatusOK, resp)
}
