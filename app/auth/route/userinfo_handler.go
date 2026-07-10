package route

import (
	"net/http"
	"strings"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// bearerPrefix is the required prefix of the Authorization header
// value carrying a bearer access token (RFC 6750 2.1).
const bearerPrefix = "Bearer "

// userInfoHandler serves GET /userinfo (OIDC Core 5.3).
type userInfoHandler struct {
	svc *service.UserInfoService
}

// handle extracts the bearer access token from the Authorization
// header, delegates to UserInfoService.UserInfo, and returns the
// resulting claims as JSON.
func (h *userInfoHandler) handle(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		writeUserInfoError(w, token.ErrInvalidToken)
		return
	}
	bearerToken := strings.TrimPrefix(authHeader, bearerPrefix)

	dto, err := h.svc.UserInfo(r.Context(), bearerToken)
	if err != nil {
		writeUserInfoError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto)
}
