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
	var transitionErr *task.TransitionError

	switch {
	case errors.Is(err, task.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "task not found"})
	case errors.Is(err, task.ErrDuplicateTitle):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "task title already exists"})
	case errors.Is(err, task.ErrEmptyTitle):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "title must not be empty"})
	case errors.Is(err, task.ErrTitleTooLong):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "title is too long"})
	case errors.Is(err, task.ErrInvalidID):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid task id"})
	case errors.Is(err, task.ErrInvalidPriority):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid priority"})
	case errors.As(err, &transitionErr):
		writeJSON(w, http.StatusConflict, errorResponse{Error: transitionErr.Error()})
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
