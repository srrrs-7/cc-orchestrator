// Package route is the presentation layer: it translates HTTP
// requests into application-layer (service) calls and translates
// their results (including domain errors) back into HTTP responses.
package route

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// errorResponse is the JSON body returned for any failed request.
type errorResponse struct {
	Error string `json:"error" validate:"required"`
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("route: encode json response", "error", err)
	}
}

// writeError inspects err and translates it into the appropriate
// HTTP status code, using errors.Is / errors.As so that wrapped
// domain errors are still recognized. Unrecognized errors are logged
// with slog and returned as a generic 500 without leaking internal
// details in the response body.
func writeError(w http.ResponseWriter, err error) {
	var (
		notFoundErr   *task.NotFoundError
		validationErr *task.ValidationError
		conflictErr   *task.ConflictError
		dbErr         *task.DBError
	)

	switch {
	case errors.As(err, &notFoundErr):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
	case errors.As(err, &validationErr):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: validationErr.Msg})
	case errors.As(err, &conflictErr):
		writeJSON(w, http.StatusConflict, errorResponse{Error: conflictErr.Msg})
	case errors.As(err, &dbErr):
		slog.Error("route: database error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	default:
		slog.Error("route: internal error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}

// writeBadRequest is used for request-parsing failures (e.g. invalid
// JSON body) that never reach the application layer.
func writeBadRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errorResponse{Error: msg})
}
