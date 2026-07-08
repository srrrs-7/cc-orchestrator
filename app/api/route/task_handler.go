package route

import (
	"encoding/json"
	"net/http"

	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// taskHandler holds the application service used to fulfill Task
// related HTTP requests.
type taskHandler struct {
	svc *service.TaskService
}

// createTaskRequest is the JSON request body for POST /tasks.
type createTaskRequest struct {
	Title string `json:"title"`
}

// taskResponse is the JSON response body representing a single Task.
type taskResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func newTaskResponse(dto service.TaskDTO) taskResponse {
	return taskResponse{
		ID:        dto.ID,
		Title:     dto.Title,
		Status:    dto.Status,
		CreatedAt: dto.CreatedAt.Format(timeLayout),
		UpdatedAt: dto.UpdatedAt.Format(timeLayout),
	}
}

// timeLayout is the timestamp format used in JSON responses.
const timeLayout = "2006-01-02T15:04:05Z07:00"

// create handles POST /tasks.
func (h *taskHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body")
		return
	}

	dto, err := h.svc.Create(r.Context(), req.Title)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newTaskResponse(dto))
}

// list handles GET /tasks.
func (h *taskHandler) list(w http.ResponseWriter, r *http.Request) {
	dtos, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	resp := make([]taskResponse, 0, len(dtos))
	for _, dto := range dtos {
		resp = append(resp, newTaskResponse(dto))
	}

	writeJSON(w, http.StatusOK, resp)
}

// get handles GET /tasks/{id}.
func (h *taskHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dto, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskResponse(dto))
}

// start handles POST /tasks/{id}/start.
func (h *taskHandler) start(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dto, err := h.svc.Start(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskResponse(dto))
}

// complete handles POST /tasks/{id}/complete.
func (h *taskHandler) complete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dto, err := h.svc.Complete(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskResponse(dto))
}
