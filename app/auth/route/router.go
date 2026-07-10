package route

import (
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// NewRouter builds the HTTP handler for the authorization server,
// wiring each OAuth 2.0 / OIDC endpoint to its handler method. It
// uses the Go 1.22+ http.ServeMux method-pattern syntax
// ("METHOD /path").
func NewRouter(authSvc *service.AuthorizationService, userInfoSvc *service.UserInfoService, discoverySvc *service.DiscoveryService) http.Handler {
	authorize := &authorizeHandler{svc: authSvc}
	tok := &tokenHandler{svc: authSvc}
	userInfo := &userInfoHandler{svc: userInfoSvc}
	discovery := &discoveryHandler{svc: discoverySvc}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /authorize", authorize.handle)
	mux.HandleFunc("POST /token", tok.handle)
	mux.HandleFunc("GET /userinfo", userInfo.handle)
	mux.HandleFunc("GET /.well-known/openid-configuration", discovery.metadata)
	mux.HandleFunc("GET /.well-known/jwks.json", discovery.jwks)

	return mux
}
