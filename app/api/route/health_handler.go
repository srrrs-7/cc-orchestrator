package route

import "net/http"

// healthResponse is the JSON body for GET /health.
type healthResponse struct {
	Status string `json:"status"`
}

// RegisterHealthRoute mounts GET /health on mux. The endpoint is
// intentionally unauthenticated so Docker HEALTHCHECK and ALB target
// group probes can verify liveness without a Bearer token (SPEC-015).
func RegisterHealthRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	})
}
