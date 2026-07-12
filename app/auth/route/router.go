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
	// AdminAPIKey is the static API key that protects the /admin/*
	// routes (ISSUE-039). When empty, admin routes are not registered
	// (fail-closed: no key → no admin access).
	AdminAPIKey string
}

// NewRouter builds the HTTP handler for the authorization server,
// wiring each OAuth 2.0 / OIDC endpoint to its handler method. It
// uses the Go 1.22+ http.ServeMux method-pattern syntax
// ("METHOD /path").
//
// introspectSvc is optional (nil = no /introspect route). When non-nil,
// POST /introspect is registered and the discovery metadata lists an
// introspection_endpoint.
//
// adminSvc is optional (nil = no admin routes). Admin routes are only
// registered when both adminSvc is non-nil and cfg.AdminAPIKey is
// non-empty; either condition being absent suppresses the /admin
// subtree entirely (fail-closed).
func NewRouter(
	authSvc *service.AuthorizationService,
	authnSvc *service.AuthenticationService,
	consentSvc *service.ConsentService,
	clients client.Repository,
	userInfoSvc *service.UserInfoService,
	discoverySvc *service.DiscoveryService,
	introspectSvc *service.IntrospectionService,
	adminSvc *service.AdminService,
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

	if introspectSvc != nil {
		introspect := &introspectHandler{svc: introspectSvc, clients: clients}
		mux.HandleFunc("POST /introspect", introspect.handle)
	}

	if adminSvc != nil && cfg.AdminAPIKey != "" {
		admin := &adminHandler{svc: adminSvc, apiKey: cfg.AdminAPIKey}
		withAuth := func(h http.HandlerFunc) http.Handler {
			return requireAdminAuth(cfg.AdminAPIKey, http.HandlerFunc(h))
		}
		mux.Handle("POST /admin/clients", withAuth(admin.handleCreateClient))
		mux.Handle("POST /admin/users", withAuth(admin.handleCreateUser))
	}

	// securityHeaders wraps every route (including /admin/*, which is
	// itself wrapped by requireAdminAuth above) so clickjacking / MIME-
	// sniffing protection applies uniformly and does not depend on
	// nginx/CloudFront being in front of this server (ISSUE-042).
	return securityHeaders(mux)
}

// SecureCookiesFromIssuer returns true when cookies should carry Secure.
func SecureCookiesFromIssuer(issuer string) bool {
	return strings.HasPrefix(strings.ToLower(issuer), "https://")
}
