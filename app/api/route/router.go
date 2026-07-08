package route

import (
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// NewRouter builds the HTTP handler for the Task API, wiring each
// route to its handler method. It uses the Go 1.22+ http.ServeMux
// method-pattern syntax ("METHOD /path").
func NewRouter(svc *service.TaskService) http.Handler {
	h := &taskHandler{svc: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", h.create)
	mux.HandleFunc("GET /tasks", h.list)
	mux.HandleFunc("GET /tasks/{id}", h.get)
	mux.HandleFunc("POST /tasks/{id}/start", h.start)
	mux.HandleFunc("POST /tasks/{id}/complete", h.complete)

	return mux
}
