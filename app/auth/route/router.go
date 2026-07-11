package route

import (
	"net/http"
	"strings"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// RouterConfig holds presentation-layer options for the authorization server.
type RouterConfig struct {
	// Issuer is the OIDC issuer URL (e.g. http://localhost:8080/auth).
	// Browser redirects use this prefix so login works behind the /auth
	// nginx proxy (SPEC-015).
	Issuer string
	// SecureCookies sets the Secure flag on IdP session cookies. Use true
	// when ISSUER is https; local http compose must keep this false.
	SecureCookies bool
}

// NewRouter builds the HTTP handler for the authorization server,
// wiring each OAuth 2.0 / OIDC endpoint to its handler method. It
// uses the Go 1.22+ http.ServeMux method-pattern syntax
// ("METHOD /path").
func NewRouter(
	authSvc *service.AuthorizationService,
	authnSvc *service.AuthenticationService,
	consentSvc *service.ConsentService,
	clients client.Repository,
	userInfoSvc *service.UserInfoService,
	discoverySvc *service.DiscoveryService,
	cfg RouterConfig,
) http.Handler {
	authorize := &authorizeHandler{svc: authSvc, authn: authnSvc, consent: consentSvc, issuer: cfg.Issuer, secureCookies: cfg.SecureCookies}
	login := newLoginHandler(authnSvc, cfg.Issuer, cfg.SecureCookies)
	consent := newConsentHandler(authSvc, authnSvc, consentSvc, cfg.Issuer, cfg.SecureCookies)
	logout := newLogoutHandler(authnSvc, clients, cfg.Issuer, cfg.SecureCookies)
	revoke := &revokeHandler{svc: authSvc}
	tok := &tokenHandler{svc: authSvc}
	userInfo := &userInfoHandler{svc: userInfoSvc}
	discovery := &discoveryHandler{svc: discoverySvc}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /authorize", authorize.handle)
	mux.HandleFunc("GET /login", login.handleGet)
	mux.HandleFunc("POST /login", login.handlePost)
	mux.HandleFunc("GET /consent", consent.handleGet)
	mux.HandleFunc("POST /consent", consent.handlePost)
	mux.HandleFunc("GET /logout", logout.handle)
	mux.HandleFunc("POST /revoke", revoke.handle)
	mux.HandleFunc("POST /token", tok.handle)
	mux.HandleFunc("GET /userinfo", userInfo.handle)
	mux.HandleFunc("GET /.well-known/openid-configuration", discovery.metadata)
	mux.HandleFunc("GET /.well-known/jwks.json", discovery.jwks)

	return mux
}

// SecureCookiesFromIssuer returns true when cookies should carry Secure.
func SecureCookiesFromIssuer(issuer string) bool {
	return strings.HasPrefix(strings.ToLower(issuer), "https://")
}
