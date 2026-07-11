package route

import (
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

type revokeHandler struct {
	svc *service.AuthorizationService
}

func (h *revokeHandler) handle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthError{Error: "invalid_request"})
		return
	}

	req := service.RevokeRequest{
		Token:         r.PostFormValue("token"),
		TokenTypeHint: r.PostFormValue("token_type_hint"),
		ClientID:      r.PostFormValue("client_id"),
	}

	if err := h.svc.Revoke(r.Context(), req); err != nil {
		writeJSON(w, http.StatusInternalServerError, oauthError{Error: "server_error"})
		return
	}

	w.WriteHeader(http.StatusOK)
}
