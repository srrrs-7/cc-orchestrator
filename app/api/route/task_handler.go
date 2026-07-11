package route

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// taskHandler holds the application service used to fulfill Task
// related HTTP requests.
type taskHandler struct {
	svc *service.TaskService
}

// createTaskRequest is the JSON request body for POST /tasks.
type createTaskRequest struct {
	Title string `json:"title" validate:"required"`
	// Priority is optional; an empty value defaults to medium
	// (applied by service.TaskService.Create).
	Priority string `json:"priority" enums:"low,medium,high"`
}

// changePriorityRequest is the JSON request body for
// POST /tasks/{id}/priority.
type changePriorityRequest struct {
	Priority string `json:"priority" validate:"required" enums:"low,medium,high"`
}

// taskResponse is the JSON response body representing a single Task.
// It is shared by every Task-returning handler (list / get / create /
// start / complete / changePriority) via newTaskResponse, so adding a
// field here propagates to all of them at once.
type taskResponse struct {
	ID        string `json:"id" validate:"required"`
	Title     string `json:"title" validate:"required"`
	Status    string `json:"status" validate:"required" enums:"todo,doing,done"`
	Priority  string `json:"priority" validate:"required" enums:"low,medium,high"`
	CreatedAt string `json:"created_at" validate:"required"`
	UpdatedAt string `json:"updated_at" validate:"required"`
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

// taskListResponse is the JSON response body for GET /tasks
// (SPEC-008): an envelope carrying a page of tasks plus the paging
// metadata the server actually applied. Total is the store's total
// task count (independent of the page window); Limit/Offset echo the
// applied (post default/clamp) values, not necessarily the raw query
// parameters the caller sent.
type taskListResponse struct {
	Items  []taskResponse `json:"items"  validate:"required"`
	Total  int            `json:"total"  validate:"required"`
	Limit  int            `json:"limit"  validate:"required"`
	Offset int            `json:"offset" validate:"required"`
}

func newTaskListResponse(dto service.TaskListDTO) taskListResponse {
	items := make([]taskResponse, 0, len(dto.Items))
	for _, item := range dto.Items {
		items = append(items, newTaskResponse(item))
	}
	return taskListResponse{
		Items:  items,
		Total:  dto.Total,
		Limit:  dto.Limit,
		Offset: dto.Offset,
	}
}

// parseQueryInt parses the named query parameter as an int. It
// returns a nil *int (meaning "unspecified") when the parameter is
// absent or empty, so that service.TaskService.List can apply
// task.Page's defaults (task.NewPage). A present-but-non-integer
// value is reported via the returned error, which the caller (list)
// translates into a 400 response.
func parseQueryInt(r *http.Request, name string) (*int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// create handles POST /tasks.
//
// @Summary      Create a task
// @Description  Creates a new task with the given title and an optional priority. An omitted or empty priority defaults to medium.
// @Tags         tasks
// @Security     BearerAuth
// @Produce      json
// @Param        request  body      createTaskRequest  true  "Task to create"
// @Success      201      {object}  taskResponse
// @Failure      400      {object}  errorResponse  "invalid body, empty title, title too long, or invalid priority"
// @Failure      401      {object}  errorResponse  "missing or invalid Authorization header"
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
// @Description  Returns a page of tasks ordered by creation time (created_at, id ascending). limit defaults to 20 and is clamped to a maximum of 100; offset defaults to 0.
// @Tags         tasks
// @Security     BearerAuth
// @Produce      json
// @Param        limit   query     int  false  "Maximum number of tasks to return (default 20, max 100; values above 100 are clamped)"
// @Param        offset  query     int  false  "Number of tasks to skip (default 0)"
// @Success      200  {object}  taskListResponse
// @Failure      400  {object}  errorResponse  "limit or offset is not an integer, limit is less than 1, or offset is negative"
// @Failure      401  {object}  errorResponse  "missing or invalid Authorization header"
// @Failure      500  {object}  errorResponse
// @Router       /tasks [get]
func (h *taskHandler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := parseQueryInt(r, "limit")
	if err != nil {
		writeBadRequest(w, "limit must be an integer")
		return
	}
	offset, err := parseQueryInt(r, "offset")
	if err != nil {
		writeBadRequest(w, "offset must be an integer")
		return
	}

	dto, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskListResponse(dto))
}

// get handles GET /tasks/{id}.
//
// @Summary      Get a task
// @Description  Returns a single task by id.
// @Tags         tasks
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Success      200  {object}  taskResponse
// @Failure      401  {object}  errorResponse  "missing or invalid Authorization header"
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
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Success      200  {object}  taskResponse
// @Failure      401  {object}  errorResponse  "missing or invalid Authorization header"
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
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Task ID"
// @Success      200  {object}  taskResponse
// @Failure      401  {object}  errorResponse  "missing or invalid Authorization header"
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
// @Security     BearerAuth
// @Produce      json
// @Param        id       path      string                 true  "Task ID"
// @Param        request  body      changePriorityRequest  true  "New priority"
// @Success      200      {object}  taskResponse
// @Failure      400      {object}  errorResponse  "invalid body or invalid priority"
// @Failure      401      {object}  errorResponse  "missing or invalid Authorization header"
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
