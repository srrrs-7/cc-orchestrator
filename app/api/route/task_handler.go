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
	// Priority is optional; an empty value defaults to medium
	// (applied by service.TaskService.Create).
	Priority string `json:"priority"`
}

// changePriorityRequest is the JSON request body for
// POST /tasks/{id}/priority.
type changePriorityRequest struct {
	Priority string `json:"priority"`
}

// taskResponse is the JSON response body representing a single Task.
// It is shared by every Task-returning handler (list / get / create /
// start / complete / changePriority) via newTaskResponse, so adding a
// field here propagates to all of them at once.
type taskResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func newTaskResponse(dto service.TaskDTO) taskResponse {
	return taskResponse{
		ID:        dto.ID,
		Title:     dto.Title,
		Status:    dto.Status,
		Priority:  dto.Priority,
		CreatedAt: dto.CreatedAt.Format(timeLayout),
		UpdatedAt: dto.UpdatedAt.Format(timeLayout),
	}
}

// timeLayout is the timestamp format used in JSON responses.
const timeLayout = "2006-01-02T15:04:05Z07:00"

// create handles POST /tasks.
//
// @Summary      Create a task
// @Description  Creates a new task with the given title and an optional priority. An omitted or empty priority defaults to medium.
// @Tags         tasks
// @Produce      json
// @Param        request  body      createTaskRequest  true  "Task to create"
// @Success      201      {object}  taskResponse
// @Failure      400      {object}  errorResponse  "invalid body, empty title, title too long, or invalid priority"
// @Failure      409      {object}  errorResponse  "a task with the same title already exists"
// @Failure      500      {object}  errorResponse
// @Router       /tasks [post]
func (h *taskHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body")
		return
	}

	dto, err := h.svc.Create(r.Context(), req.Title, req.Priority)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newTaskResponse(dto))
}

// list handles GET /tasks.
//
// @Summary      List tasks
// @Description  Returns every task.
// @Tags         tasks
// @Produce      json
// @Success      200  {array}   taskResponse
// @Failure      500  {object}  errorResponse
// @Router       /tasks [get]
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
//
// @Summary      Get a task
// @Description  Returns a single task by id.
// @Tags         tasks
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Success      200  {object}  taskResponse
// @Failure      404  {object}  errorResponse  "task does not exist"
// @Failure      500  {object}  errorResponse
// @Router       /tasks/{id} [get]
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
//
// @Summary      Start a task
// @Description  Transitions a task from todo to doing.
// @Tags         tasks
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Success      200  {object}  taskResponse
// @Failure      404  {object}  errorResponse  "task does not exist"
// @Failure      409  {object}  errorResponse  "task is not in a state that can transition to doing"
// @Failure      500  {object}  errorResponse
// @Router       /tasks/{id}/start [post]
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
//
// @Summary      Complete a task
// @Description  Transitions a task from doing to done.
// @Tags         tasks
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Success      200  {object}  taskResponse
// @Failure      404  {object}  errorResponse  "task does not exist"
// @Failure      409  {object}  errorResponse  "task is not in a state that can transition to done"
// @Failure      500  {object}  errorResponse
// @Router       /tasks/{id}/complete [post]
func (h *taskHandler) complete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dto, err := h.svc.Complete(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskResponse(dto))
}

// changePriority handles POST /tasks/{id}/priority.
//
// @Summary      Change a task's priority
// @Description  Updates a task's priority (low, medium, or high) without altering its status.
// @Tags         tasks
// @Produce      json
// @Param        id       path      string                 true  "Task ID"
// @Param        request  body      changePriorityRequest  true  "New priority"
// @Success      200      {object}  taskResponse
// @Failure      400      {object}  errorResponse  "invalid body or invalid priority"
// @Failure      404      {object}  errorResponse  "task does not exist"
// @Failure      500      {object}  errorResponse
// @Router       /tasks/{id}/priority [post]
func (h *taskHandler) changePriority(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req changePriorityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body")
		return
	}

	dto, err := h.svc.ChangePriority(r.Context(), id, req.Priority)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskResponse(dto))
}
