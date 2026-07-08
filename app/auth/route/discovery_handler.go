package route

import (
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// discoveryHandler serves the two OIDC/JWK well-known discovery
// documents: /.well-known/openid-configuration (OIDC Discovery 1.0)
// and /.well-known/jwks.json (RFC 7517).
type discoveryHandler struct {
	svc *service.DiscoveryService
}

// metadata handles GET /.well-known/openid-configuration.
func (h *discoveryHandler) metadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Metadata(r.Context()))
}

// jwks handles GET /.well-known/jwks.json.
func (h *discoveryHandler) jwks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.JWKS(r.Context()))
}
