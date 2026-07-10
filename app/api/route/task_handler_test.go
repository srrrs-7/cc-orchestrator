package route_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/api/route"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// newTestHandler wires a fresh in-memory-repository-backed router so
// each test case starts from an empty store.
func newTestHandler() http.Handler {
	repo := memory.NewTaskRepository()
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, dupChk)
	return route.NewRouter(svc)
}

// failingRepository is a task.Repository whose every method returns a
// generic, non-sentinel error. It exists solely to drive route's
// default writeError branch (500 errorResponse), a wire-contract
// scenario the happy-path memory.TaskRepository cannot produce.
type failingRepository struct{}

var _ task.Repository = failingRepository{}

func (failingRepository) Save(context.Context, *task.Task) error {
	return errors.New("failingRepository: save always fails")
}

func (failingRepository) FindByID(context.Context, task.ID) (*task.Task, error) {
	return nil, errors.New("failingRepository: find by id always fails")
}

func (failingRepository) FindByTitle(context.Context, task.Title) (*task.Task, error) {
	return nil, errors.New("failingRepository: find by title always fails")
}

func (failingRepository) ListPage(context.Context, task.Page) ([]*task.Task, int, error) {
	return nil, 0, errors.New("failingRepository: list page always fails")
}

// newFailingTestHandler wires a router backed by failingRepository,
// for wire-contract cases that must observe a 500 errorResponse.
func newFailingTestHandler() http.Handler {
	repo := failingRepository{}
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, dupChk)
	return route.NewRouter(svc)
}

// dbErrorRepository is a task.Repository whose every method returns a
// *task.DBError (ISSUE-018's category type for infrastructure-layer
// failures). Unlike failingRepository (which returns a generic,
// non-category error to drive route's default 500 branch),
// dbErrorRepository exists to pin that the DBError *category*
// independently maps to 500 through writeError's errors.As switch,
// rather than only reaching 500 via the fallback default case.
type dbErrorRepository struct{}

var _ task.Repository = dbErrorRepository{}

func (dbErrorRepository) Save(context.Context, *task.Task) error {
	return task.NewDBError(errors.New("dbErrorRepository: save always fails"))
}

func (dbErrorRepository) FindByID(context.Context, task.ID) (*task.Task, error) {
	return nil, task.NewDBError(errors.New("dbErrorRepository: find by id always fails"))
}

func (dbErrorRepository) FindByTitle(context.Context, task.Title) (*task.Task, error) {
	return nil, task.NewDBError(errors.New("dbErrorRepository: find by title always fails"))
}

func (dbErrorRepository) ListPage(context.Context, task.Page) ([]*task.Task, int, error) {
	return nil, 0, task.NewDBError(errors.New("dbErrorRepository: list page always fails"))
}

// newDBErrorTestHandler wires a router backed by dbErrorRepository.
func newDBErrorTestHandler() http.Handler {
	repo := dbErrorRepository{}
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, dupChk)
	return route.NewRouter(svc)
}

type taskResponseBody struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type errorResponseBody struct {
	Error string `json:"error"`
}

func doRequest(t *testing.T, h http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, target, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// doRawRequest is like doRequest but sends body verbatim (unmarshaled),
// so callers can exercise malformed-JSON request bodies.
func doRawRequest(t *testing.T, h http.Handler, method, target, rawBody string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(rawBody))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeTask(t *testing.T, rec *httptest.ResponseRecorder) taskResponseBody {
	t.Helper()
	var got taskResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorResponseBody {
	t.Helper()
	var got errorResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode error body: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

func TestCreateTask_Success(t *testing.T) {
	h := newTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	got := decodeTask(t, rec)
	if got.Title != "buy milk" {
		t.Errorf("Title = %q, want %q", got.Title, "buy milk")
	}
	if got.Status != "todo" {
		t.Errorf("Status = %q, want %q", got.Status, "todo")
	}
	// R2: omitting priority in the request body defaults to medium.
	if got.Priority != "medium" {
		t.Errorf("Priority = %q, want %q", got.Priority, "medium")
	}
	if got.ID == "" {
		t.Error("ID is empty, want non-empty")
	}
}

// TestCreateTask_WithPriority covers R2 (an explicit priority in the
// request body is honored) and R4 (the created response carries the
// snake_case priority field with the exact value requested).
func TestCreateTask_WithPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority string
	}{
		{name: "low", priority: "low"},
		{name: "medium", priority: "medium"},
		{name: "high", priority: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler()

			rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": tt.priority})

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusCreated, rec.Body.String())
			}
			got := decodeTask(t, rec)
			if got.Priority != tt.priority {
				t.Errorf("Priority = %q, want %q", got.Priority, tt.priority)
			}
		})
	}
}

// TestCreateTask_InvalidPriority covers R5: an unknown priority value
// in the create request must be rejected with 400, not silently
// defaulted or accepted.
func TestCreateTask_InvalidPriority(t *testing.T) {
	h := newTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "urgent"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestCreateTask_ExplicitNullPriority covers the boundary flagged in
// the plan's risk section: an explicitly-sent JSON null for priority
// (as opposed to an omitted field or an explicit "") must decode to
// Go's zero value for string ("") without a decode error, and
// therefore also default to medium (R2), exactly like an omitted or
// empty priority.
func TestCreateTask_ExplicitNullPriority(t *testing.T) {
	h := newTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]any{"title": "buy milk", "priority": nil})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusCreated, rec.Body.String())
	}
	got := decodeTask(t, rec)
	if got.Priority != "medium" {
		t.Errorf("Priority = %q, want %q", got.Priority, "medium")
	}
}

func TestCreateTask_DuplicateTitle(t *testing.T) {
	h := newTestHandler()

	first := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"})
	if first.Code != http.StatusCreated {
		t.Fatalf("setup: status = %d, want %d (body=%q)", first.Code, http.StatusCreated, first.Body.String())
	}

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestCreateTask_EmptyTitle(t *testing.T) {
	h := newTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

func TestGetTask_NotFound(t *testing.T) {
	h := newTestHandler()

	rec := doRequest(t, h, http.MethodGet, "/tasks/does-not-exist", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

func TestGetTask_Success(t *testing.T) {
	h := newTestHandler()

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "high"}))

	rec := doRequest(t, h, http.MethodGet, "/tasks/"+created.ID, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeTask(t, rec)
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	// R4: priority set at creation survives the GET round trip.
	if got.Priority != "high" {
		t.Errorf("Priority = %q, want %q", got.Priority, "high")
	}
}

func TestCompleteTask_InvalidTransitionFromTodo(t *testing.T) {
	h := newTestHandler()

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"}))

	rec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/complete", nil)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

func TestStartThenCompleteTask_Success(t *testing.T) {
	h := newTestHandler()

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "high"}))

	startRec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/start", nil)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start: status = %d, want %d (body=%q)", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	if got := decodeTask(t, startRec); got.Status != "doing" {
		t.Errorf("start: Status = %q, want %q", got.Status, "doing")
	} else if got.Priority != "high" {
		// Non-functional requirement: state transitions must not
		// disturb priority (orthogonality).
		t.Errorf("start: Priority = %q, want unchanged %q", got.Priority, "high")
	}

	completeRec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/complete", nil)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("complete: status = %d, want %d (body=%q)", completeRec.Code, http.StatusOK, completeRec.Body.String())
	}
	if got := decodeTask(t, completeRec); got.Status != "done" {
		t.Errorf("complete: Status = %q, want %q", got.Status, "done")
	} else if got.Priority != "high" {
		t.Errorf("complete: Priority = %q, want unchanged %q", got.Priority, "high")
	}
}

// taskListResponseBody mirrors route.taskListResponse (SPEC-008's
// envelope) for decoding in tests.
type taskListResponseBody struct {
	Items  []taskResponseBody `json:"items"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

func TestListTasks(t *testing.T) {
	h := newTestHandler()

	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "low"})
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "walk dog", "priority": "high"})

	rec := doRequest(t, h, http.MethodGet, "/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got taskListResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got.Items) != 2 {
		t.Errorf("len(got.Items) = %d, want 2", len(got.Items))
	}
	// R1/R2: an unspecified limit/offset default to 20/0, echoed in
	// the envelope, and total reflects the store's full count.
	if got.Total != 2 {
		t.Errorf("Total = %d, want 2", got.Total)
	}
	if got.Limit != 20 {
		t.Errorf("Limit = %d, want 20", got.Limit)
	}
	if got.Offset != 0 {
		t.Errorf("Offset = %d, want 0", got.Offset)
	}

	// R4: every item in the list response carries a non-empty
	// snake_case priority field.
	for _, item := range got.Items {
		if item.Priority == "" {
			t.Errorf("item %q: Priority is empty, want set", item.Title)
		}
	}
}

// TestListTasks_InvalidQueryParams covers R3's validation path: a
// non-integer limit/offset, or a limit/offset that fails task.Page's
// domain validation (limit < 1, negative offset), must be rejected
// with 400 and the existing error envelope {error}, never reaching a
// 200 response.
func TestListTasks_InvalidQueryParams(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "non-integer limit", query: "?limit=abc"},
		{name: "non-integer offset", query: "?offset=xyz"},
		{name: "limit is zero (below the domain's minimum of 1)", query: "?limit=0"},
		{name: "limit is negative", query: "?limit=-1"},
		{name: "offset is negative", query: "?offset=-1"},
		{name: "limit is a float, not an integer", query: "?limit=1.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler()

			rec := doRequest(t, h, http.MethodGet, "/tasks"+tt.query, nil)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if body := decodeError(t, rec); body.Error == "" {
				t.Error("error response body is empty, want a message")
			}
		})
	}
}

// TestListTasks_LimitClampedAboveMax covers R3: a limit above
// task.MaxLimit (100) is clamped rather than rejected -- the request
// still succeeds (200), and the envelope's echoed limit reflects the
// clamp (100), not the raw requested value (1000).
func TestListTasks_LimitClampedAboveMax(t *testing.T) {
	h := newTestHandler()
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"})

	rec := doRequest(t, h, http.MethodGet, "/tasks?limit=1000", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got taskListResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.Limit != 100 {
		t.Errorf("Limit = %d, want 100 (clamped)", got.Limit)
	}
	if got.Offset != 0 {
		t.Errorf("Offset = %d, want 0", got.Offset)
	}
}

// TestListTasks_ExplicitLimitOffsetWindow covers R1/R2: an explicit
// limit/offset pair within range is applied to the returned window
// (not just echoed), and total still reflects the store's full count
// regardless of the window.
func TestListTasks_ExplicitLimitOffsetWindow(t *testing.T) {
	h := newTestHandler()
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "task a"})
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "task b"})
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "task c"})

	rec := doRequest(t, h, http.MethodGet, "/tasks?limit=1&offset=1", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got taskListResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1 (limit=1 must cap the window)", len(got.Items))
	}
	if got.Total != 3 {
		t.Errorf("Total = %d, want 3 (total is independent of the limit/offset window)", got.Total)
	}
	if got.Limit != 1 {
		t.Errorf("Limit = %d, want 1", got.Limit)
	}
	if got.Offset != 1 {
		t.Errorf("Offset = %d, want 1", got.Offset)
	}
}

// TestListTasks_OffsetBeyondTotal covers the documented boundary
// (SPEC-008 plan §"検証/クランプ仕様"): an offset at or beyond the
// store's total yields an empty items slice with 200, not an error,
// and total still reports the store's full count.
func TestListTasks_OffsetBeyondTotal(t *testing.T) {
	h := newTestHandler()
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "task a"})

	rec := doRequest(t, h, http.MethodGet, "/tasks?offset=50", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got taskListResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got.Items) != 0 {
		t.Errorf("len(Items) = %d, want 0", len(got.Items))
	}
	if got.Total != 1 {
		t.Errorf("Total = %d, want 1", got.Total)
	}
}

// TestChangePriority_Success covers R3: POST /tasks/{id}/priority
// updates the priority and returns 200 with the full task
// representation reflecting the new value, without altering status.
func TestChangePriority_Success(t *testing.T) {
	h := newTestHandler()

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "low"}))

	rec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/priority", map[string]string{"priority": "high"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeTask(t, rec)
	if got.Priority != "high" {
		t.Errorf("Priority = %q, want %q", got.Priority, "high")
	}
	if got.Status != "todo" {
		t.Errorf("Status = %q, want unchanged %q", got.Status, "todo")
	}
}

// TestChangePriority_InvalidPriority covers R5: an unknown priority
// value sent to POST /tasks/{id}/priority is rejected with 400.
func TestChangePriority_InvalidPriority(t *testing.T) {
	h := newTestHandler()

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"}))

	rec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/priority", map[string]string{"priority": "urgent"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestChangePriority_ExplicitNullPriority covers the boundary flagged
// in the plan's risk section, for the change-priority endpoint rather
// than creation: an explicitly-sent JSON null for priority decodes to
// Go's zero value for string (""). Unlike creation, changing priority
// has no "unspecified means default" concept (service.ChangePriority
// calls the strict task.ParsePriority directly), so the empty string
// must be rejected with 400 (ErrInvalidPriority), not silently
// defaulted.
func TestChangePriority_ExplicitNullPriority(t *testing.T) {
	h := newTestHandler()

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "low"}))

	rec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/priority", map[string]any{"priority": nil})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestChangePriority_NotFound covers the boundary shared with
// start/complete: an unknown task id yields 404, matching the
// existing action-style endpoints.
func TestChangePriority_NotFound(t *testing.T) {
	h := newTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/priority", map[string]string{"priority": "high"})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestDBErrorCategory_MapsTo500 covers ISSUE-018: a *task.DBError
// returned from the repository must be recognized by writeError's
// errors.As type switch (the DBError case), independently of the
// unrecognized-error default branch that failingRepository above
// drives. Both branches happen to produce the same 500 + generic body
// (per the plan's documented, accepted ambiguity), so this test's
// value is pinning that the DBError *category* itself is wired to
// 500 -- if a future change moved DBError to, say, a 400 case by
// mistake, this test (unlike the failingRepository-driven ones) would
// catch it even though failingRepository's default-path assertions
// would not move.
func TestDBErrorCategory_MapsTo500(t *testing.T) {
	tests := []struct {
		name string
		do   func(t *testing.T, h http.Handler) *httptest.ResponseRecorder
	}{
		{
			name: "POST /tasks (FindByTitle failure via DuplicateChecker)",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "db error task"})
			},
		},
		{
			name: "GET /tasks (ListPage failure)",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodGet, "/tasks", nil)
			},
		},
		{
			name: "GET /tasks/{id} (FindByID failure)",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodGet, "/tasks/some-id", nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDBErrorTestHandler()

			rec := tt.do(t, h)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusInternalServerError, rec.Body.String())
			}
			body := decodeError(t, rec)
			if body.Error == "" {
				t.Error("error response body is empty, want a message")
			}
			// The DBError's underlying cause must never leak into the
			// client-facing body (it is logged via slog instead); the
			// body must be the generic, fixed message.
			if body.Error != "internal server error" {
				t.Errorf("error message = %q, want the generic %q (DBError internals must not leak to the client)", body.Error, "internal server error")
			}
		})
	}
}

// ---------------------------------------------------------------------
// Wire-contract tests (SPEC-003 T2 / R2).
//
// The tests above pin *behavior* (business-rule correctness: exact
// field values, transition semantics, duplicate detection, ...). The
// tests below instead pin the *wire shape* that the generated OpenAPI
// document must describe for every endpoint: the exact success-body
// field set and casing (route.taskResponse), the exact error-body
// shape (route.errorResponse), and the status code the implementation
// actually produces for each scenario. They deliberately do not
// re-assert field *values* already covered above.
// ---------------------------------------------------------------------

// wireTaskFields is the exact, snake_case field set of
// route.taskResponse. Every field must be a JSON string: in
// particular created_at/updated_at, since the swag annotations
// reference taskResponse (all-string) rather than the
// time.Time-typed service.TaskDTO.
var wireTaskFields = []string{"id", "title", "status", "priority", "created_at", "updated_at"}

// wireErrorFields is the exact field set of route.errorResponse.
var wireErrorFields = []string{"error"}

// decodeMap decodes rec's body as a generic JSON object, preserving
// the actual field set and value types (unlike decoding into a typed
// struct, which silently tolerates missing/extra fields and coerces
// nothing).
func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body as object: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

// assertWireShape verifies body has exactly wantFields as its key set
// (no missing, no extra) and that every one of those fields is a JSON
// string.
func assertWireShape(t *testing.T, body map[string]any, wantFields []string) {
	t.Helper()

	got := make([]string, 0, len(body))
	for k := range body {
		got = append(got, k)
	}
	slices.Sort(got)

	want := slices.Clone(wantFields)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("field set = %v, want exactly %v", got, want)
	}

	for _, f := range wantFields {
		v, ok := body[f]
		if !ok {
			continue // already reported by the field-set check above
		}
		if _, isString := v.(string); !isString {
			t.Errorf("field %q = %T(%v), want a JSON string", f, v, v)
		}
	}
}

// TestWireContract_TaskAndErrorShapes covers every Task-returning
// endpoint (create/get/start/complete/changePriority) and its
// documented 400/404/409/500 failure modes, pinning:
//   - success bodies are exactly {id,title,status,priority,
//     created_at,updated_at}, snake_case, all-string (taskResponse);
//   - failure bodies are exactly {error} (errorResponse);
//   - the status code produced for each scenario.
//
// GET /tasks (a JSON array, not a single object) is covered
// separately by TestWireContract_ListResponseFields.
func TestWireContract_TaskAndErrorShapes(t *testing.T) {
	tests := []struct {
		name           string
		useFailingRepo bool
		do             func(t *testing.T, h http.Handler) *httptest.ResponseRecorder
		wantStatus     int
		wantFields     []string
	}{
		// POST /tasks
		{
			name: "POST /tasks success -> 201 taskResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task", "priority": "high"})
			},
			wantStatus: http.StatusCreated,
			wantFields: wireTaskFields,
		},
		{
			name: "POST /tasks malformed JSON body -> 400 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRawRequest(t, h, http.MethodPost, "/tasks", "{not-json")
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks empty title -> 400 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": ""})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks invalid priority -> 400 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task", "priority": "urgent"})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks duplicate title -> 409 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire dup"})
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire dup"})
			},
			wantStatus: http.StatusConflict,
			wantFields: wireErrorFields,
		},
		{
			name:           "POST /tasks repository failure -> 500 errorResponse",
			useFailingRepo: true,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"})
			},
			wantStatus: http.StatusInternalServerError,
			wantFields: wireErrorFields,
		},

		// GET /tasks/{id}
		{
			name: "GET /tasks/{id} success -> 200 taskResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"}))
				return doRequest(t, h, http.MethodGet, "/tasks/"+created.ID, nil)
			},
			wantStatus: http.StatusOK,
			wantFields: wireTaskFields,
		},
		{
			name: "GET /tasks/{id} not found -> 404 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodGet, "/tasks/does-not-exist", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/start
		{
			name: "POST /tasks/{id}/start success -> 200 taskResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"}))
				return doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/start", nil)
			},
			wantStatus: http.StatusOK,
			wantFields: wireTaskFields,
		},
		{
			name: "POST /tasks/{id}/start not found -> 404 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/start", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks/{id}/start invalid transition -> 409 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"}))
				doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/start", nil)
				return doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/start", nil)
			},
			wantStatus: http.StatusConflict,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/complete
		{
			name: "POST /tasks/{id}/complete success -> 200 taskResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"}))
				doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/start", nil)
				return doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/complete", nil)
			},
			wantStatus: http.StatusOK,
			wantFields: wireTaskFields,
		},
		{
			name: "POST /tasks/{id}/complete not found -> 404 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/complete", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks/{id}/complete invalid transition -> 409 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"}))
				return doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/complete", nil)
			},
			wantStatus: http.StatusConflict,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/priority
		{
			name: "POST /tasks/{id}/priority success -> 200 taskResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task", "priority": "low"}))
				return doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/priority", map[string]string{"priority": "high"})
			},
			wantStatus: http.StatusOK,
			wantFields: wireTaskFields,
		},
		{
			name: "POST /tasks/{id}/priority malformed JSON body -> 400 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"}))
				return doRawRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/priority", "{not-json")
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks/{id}/priority invalid priority value -> 400 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"}))
				return doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/priority", map[string]string{"priority": "urgent"})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks/{id}/priority not found -> 404 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/priority", map[string]string{"priority": "high"})
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h http.Handler
			if tt.useFailingRepo {
				h = newFailingTestHandler()
			} else {
				h = newTestHandler()
			}

			rec := tt.do(t, h)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			assertWireShape(t, decodeMap(t, rec), tt.wantFields)
		})
	}
}

// wireListEnvelopeFields is the exact field set of
// route.taskListResponse (SPEC-008's envelope), excluding "items"
// itself (its element shape is pinned separately via wireTaskFields).
var wireListEnvelopeFields = []string{"items", "total", "limit", "offset"}

// TestWireContract_ListResponseFields pins GET /tasks's shape
// (SPEC-008): a JSON envelope object {items,total,limit,offset}
// whose every items[] element is a taskResponse (the same exact
// field set and casing pinned above for the single-object
// endpoints).
func TestWireContract_ListResponseFields(t *testing.T) {
	h := newTestHandler()
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire list task 1", "priority": "low"})
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire list task 2", "priority": "high"})

	rec := doRequest(t, h, http.MethodGet, "/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	envelope := decodeMap(t, rec)
	gotFields := make([]string, 0, len(envelope))
	for k := range envelope {
		gotFields = append(gotFields, k)
	}
	slices.Sort(gotFields)
	wantFields := slices.Clone(wireListEnvelopeFields)
	slices.Sort(wantFields)
	if !slices.Equal(gotFields, wantFields) {
		t.Fatalf("envelope field set = %v, want exactly %v", gotFields, wantFields)
	}

	items, ok := envelope["items"].([]any)
	if !ok {
		t.Fatalf("items = %T(%v), want a JSON array", envelope["items"], envelope["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("item = %T(%v), want a JSON object", item, item)
		}
		assertWireShape(t, itemMap, wireTaskFields)
	}
}
