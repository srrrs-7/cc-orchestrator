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
	"github.com/srrrs-7/cc-orchestrator/app/api/route"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// failingRepository is a task.Repository whose every method returns a
// generic, non-sentinel error. It exists solely to drive route's
// default writeError branch (500 errorResponse), a wire-contract
// scenario a functional store cannot produce.
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
	svc := service.NewTaskService(repo, repo, dupChk)
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
	svc := service.NewTaskService(repo, repo, dupChk)
	return route.NewRouter(svc)
}

// notFoundRepository is a task.Repository whose find methods always
// return *task.NotFoundError (unwrapping to task.ErrNotFound). It
// drives the route layer's 404 branch without needing a live store or
// pre-existing data, for tests that only care about the not-found
// error path. This is a minimal, targeted stub -- not a general-purpose
// in-memory store -- consistent with SPEC-011's prohibition on
// re-implementing infra/memory.
type notFoundRepository struct{}

var _ task.Repository = notFoundRepository{}

func (notFoundRepository) Save(_ context.Context, _ *task.Task) error { return nil }
func (notFoundRepository) FindByID(_ context.Context, _ task.ID) (*task.Task, error) {
	return nil, task.NewNotFoundError()
}
func (notFoundRepository) FindByTitle(_ context.Context, _ task.Title) (*task.Task, error) {
	return nil, task.NewNotFoundError()
}
func (notFoundRepository) ListPage(_ context.Context, _ task.Page) ([]*task.Task, int, error) {
	return nil, 0, nil
}

// newNotFoundTestHandler wires a router backed by notFoundRepository,
// for wire-contract cases that must observe a 404 errorResponse without
// requiring any pre-existing store state.
func newNotFoundTestHandler() http.Handler {
	repo := notFoundRepository{}
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, repo, dupChk)
	return route.NewRouter(svc)
}

// --- shared request/response helpers ------------------------------------

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

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorResponseBody {
	t.Helper()
	var got errorResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode error body: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

// --- untagged (offline) unit tests: validation + error injection --------
//
// These tests exercise paths the route layer can decide without a
// functional store: input validation that fails before any repository
// call (invalid priority, empty title, malformed JSON, invalid query
// params) and error injection via the stub repositories above
// (failingRepository drives the 500 default branch,
// dbErrorRepository pins the DBError category, notFoundRepository
// drives the 404 branch). No real DB or in-memory store is required.

// TestCreateTask_InvalidPriority covers R5: an unknown priority value
// in the create request must be rejected with 400.  Priority validation
// (service.Create -> task.ParsePriority) fires before any store access,
// so failingRepository's errors are never surfaced.
func TestCreateTask_InvalidPriority(t *testing.T) {
	h := newFailingTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "urgent"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestCreateTask_EmptyTitle covers the 異常系: an empty title is
// rejected with 400 before any store access (service.Create ->
// task.NewTitle).
func TestCreateTask_EmptyTitle(t *testing.T) {
	h := newFailingTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestGetTask_NotFound covers the not-found path: GET /tasks/{id} for
// a nonexistent ID must return 404. notFoundRepository returns
// *task.NotFoundError so the route layer maps it to 404 without a live
// store or pre-existing data.
func TestGetTask_NotFound(t *testing.T) {
	h := newNotFoundTestHandler()

	rec := doRequest(t, h, http.MethodGet, "/tasks/does-not-exist", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestListTasks_InvalidQueryParams covers R3's validation path: a
// non-integer limit/offset, or a limit/offset that fails task.Page's
// domain validation (limit < 1, negative offset), must be rejected
// with 400 and the existing error envelope {error}, never reaching a
// 200 response. The handler and service validate these before any
// store call, so failingRepository's errors are never surfaced.
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
			h := newFailingTestHandler()

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

// TestChangePriority_InvalidPriority covers R5: an unknown priority
// value sent to POST /tasks/{id}/priority is rejected with 400.
// service.ChangePriority validates priority (task.ParsePriority) before
// fetching the task from the store, so no pre-existing task is needed
// and failingRepository's errors are never surfaced.
func TestChangePriority_InvalidPriority(t *testing.T) {
	h := newFailingTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks/some-id/priority", map[string]string{"priority": "urgent"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestChangePriority_ExplicitNullPriority covers the boundary flagged
// in the plan's risk section, for the change-priority endpoint: an
// explicitly-sent JSON null for priority decodes to Go's zero value for
// string (""). service.ChangePriority calls task.ParsePriority directly
// (strict) before any store access, so the empty string must be
// rejected with 400 (ErrInvalidPriority) without needing a live store.
func TestChangePriority_ExplicitNullPriority(t *testing.T) {
	h := newFailingTestHandler()

	rec := doRequest(t, h, http.MethodPost, "/tasks/some-id/priority", map[string]any{"priority": nil})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestChangePriority_NotFound covers the boundary shared with
// start/complete: a valid-priority request for an unknown task id yields
// 404. service.ChangePriority validates priority first (task.ParsePriority),
// then calls FindByID which notFoundRepository answers with
// *task.NotFoundError → 404.
func TestChangePriority_NotFound(t *testing.T) {
	h := newNotFoundTestHandler()

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
// Wire-contract tests (SPEC-003 T2 / R2) -- offline (untagged) half.
//
// This half covers error paths that do not require functional store
// state: input validation errors (malformed JSON, empty title, invalid
// priority), repository injection failures (500), and not-found paths
// (404 via notFoundRepository). Success paths and state-dependent
// error paths (duplicate, invalid transition) are in the
// //go:build integration counterpart (task_handler_integration_test.go).
// ---------------------------------------------------------------------

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

// TestWireContract_ErrorAndValidationShapes covers every error/
// validation path that does not require functional store state: malformed
// JSON, empty title, invalid priority (400s), repository failures (500),
// and not-found paths (404). Success paths and state-dependent error
// shapes are covered in task_handler_integration_test.go.
func TestWireContract_ErrorAndValidationShapes(t *testing.T) {
	tests := []struct {
		name       string
		handler    func() http.Handler
		do         func(t *testing.T, h http.Handler) *httptest.ResponseRecorder
		wantStatus int
		wantFields []string
	}{
		// POST /tasks
		{
			name:    "POST /tasks malformed JSON body -> 400 errorResponse",
			handler: newFailingTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRawRequest(t, h, http.MethodPost, "/tasks", "{not-json")
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks empty title -> 400 errorResponse",
			handler: newFailingTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": ""})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks invalid priority -> 400 errorResponse",
			handler: newFailingTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task", "priority": "urgent"})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks repository failure -> 500 errorResponse",
			handler: newFailingTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"})
			},
			wantStatus: http.StatusInternalServerError,
			wantFields: wireErrorFields,
		},

		// GET /tasks/{id}
		{
			name:    "GET /tasks/{id} not found -> 404 errorResponse",
			handler: newNotFoundTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodGet, "/tasks/does-not-exist", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/start
		{
			name:    "POST /tasks/{id}/start not found -> 404 errorResponse",
			handler: newNotFoundTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/start", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/complete
		{
			name:    "POST /tasks/{id}/complete not found -> 404 errorResponse",
			handler: newNotFoundTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/complete", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/priority
		{
			name:    "POST /tasks/{id}/priority malformed JSON body -> 400 errorResponse",
			handler: newFailingTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRawRequest(t, h, http.MethodPost, "/tasks/some-id/priority", "{not-json")
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks/{id}/priority invalid priority value -> 400 errorResponse",
			handler: newFailingTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/some-id/priority", map[string]string{"priority": "urgent"})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks/{id}/priority not found -> 404 errorResponse",
			handler: newNotFoundTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/priority", map[string]string{"priority": "high"})
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.handler()
			rec := tt.do(t, h)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			assertWireShape(t, decodeMap(t, rec), tt.wantFields)
		})
	}
}
