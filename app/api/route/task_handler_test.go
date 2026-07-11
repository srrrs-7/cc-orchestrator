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
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/api/route"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// failingRepository is a task.Repository whose every method returns a
// generic, non-sentinel error. As of SPEC-013 (R2 exception 1), this
// is the one hand-written double deliberately kept in this package:
// infra/postgres's real repositories only ever return
// *task.NotFoundError / *task.ValidationError / *task.ConflictError /
// *task.DBError (see task_reader.go / task_writer.go), so an
// unclassified error can never actually reach route.writeError's
// default (fallback) branch through a real database -- only a stub
// like this one can drive that specific branch.
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
// for the one wire-contract case that must observe route's default
// (unclassified-error) 500 branch specifically -- see
// failingRepository's doc comment for why a real DB cannot produce
// this scenario.
func newFailingTestHandler() http.Handler {
	repo := failingRepository{}
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, repo, dupChk)
	return route.NewRouter(svc)
}

// newDBErrorTestHandler wires a router backed by a real
// postgres.TaskRepository against the api_test database, then closes
// the underlying *sql.DB before returning the handler. Every
// subsequent repository call therefore fails with a real driver error
// ("sql: database is closed"), which infra/postgres wraps as a
// *task.DBError (task_reader.go / task_writer.go's task.NewDBError
// calls) -- pinning that the DBError *category* independently maps to
// 500 through writeError's errors.As switch, rather than only
// reaching 500 via failingRepository's fallback default case above.
// This mirrors infra/postgres's own "forced driver failure (closed
// connection)" pattern
// (infra/postgres/task_repository_integration_test.go, ISSUE-018),
// applied here at the route layer instead.
func newDBErrorTestHandler(t *testing.T) http.Handler {
	t.Helper()
	db := testsupport.OpenTestDB(t)
	testsupport.TruncateTasks(t, db)
	repo := postgres.NewTaskRepository(db)
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, repo, dupChk)
	if err := db.Close(); err != nil {
		t.Fatalf("setup: close db: %v", err)
	}
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

// --- validation + error-injection tests ----------------------------------
//
// As of SPEC-013, this package is a single, untagged suite that runs
// against the real api_test database as part of the default `make
// test` / `make check` (see task_handler_integration_test.go's
// newIntegrationTestHandler for the shared real-DB wiring). The tests
// below exercise:
//   - input validation that fails before any repository call (invalid
//     priority, empty title, malformed JSON, invalid query params),
//     wired against newIntegrationTestHandler since the store is
//     never actually reached;
//   - not-found paths, wired against newIntegrationTestHandler with an
//     empty store (its real FindByID/FindByTitle return
//     *task.NotFoundError for an absent row, no fixture required);
//   - the two error-injection doubles kept for scenarios a functional
//     Postgres store cannot itself produce (failingRepository's
//     unclassified-error default branch, newDBErrorTestHandler's
//     forced driver failure) -- see their doc comments above.

// TestCreateTask_InvalidPriority covers R5: an unknown priority value
// in the create request must be rejected with 400. Priority validation
// (service.Create -> task.ParsePriority) fires before any store access.
func TestCreateTask_InvalidPriority(t *testing.T) {
	h := newIntegrationTestHandler(t)

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
	h := newIntegrationTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Error == "" {
		t.Error("error response body is empty, want a message")
	}
}

// TestGetTask_NotFound covers the not-found path: GET /tasks/{id} for
// a nonexistent ID must return 404. Against an empty api_test store,
// the real postgres.TaskReader's FindByID returns *task.NotFoundError,
// which the route layer maps to 404.
func TestGetTask_NotFound(t *testing.T) {
	h := newIntegrationTestHandler(t)

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
// store call.
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
			h := newIntegrationTestHandler(t)

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
// fetching the task from the store, so no pre-existing task is needed.
func TestChangePriority_InvalidPriority(t *testing.T) {
	h := newIntegrationTestHandler(t)

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
	h := newIntegrationTestHandler(t)

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
// then calls FindByID which, against an empty api_test store, answers
// with *task.NotFoundError -> 404.
func TestChangePriority_NotFound(t *testing.T) {
	h := newIntegrationTestHandler(t)

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
// would not move. As of SPEC-013, the DBError is induced from a real
// Postgres connection (closed mid-test) rather than a hand-written
// stub -- see newDBErrorTestHandler's doc comment.
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
			h := newDBErrorTestHandler(t)

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
// Wire-contract tests (SPEC-003 T2 / R2) -- validation/error half.
//
// This half covers error paths that do not require pre-existing store
// data: input validation errors (malformed JSON, empty title, invalid
// priority), the failingRepository-driven unrecognized-error 500, and
// not-found paths (404 against an empty api_test store). Success paths
// and state-dependent error paths (duplicate, invalid transition) are
// in task_handler_integration_test.go's
// TestWireContract_SuccessAndStateShapes, which shares this file's
// doRequest/doRawRequest/decodeMap/assertWireShape helpers -- both
// live in the same untagged route_test package and run together as
// part of the default `make test` (SPEC-013; there is no longer a
// separate `//go:build integration` half).
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
// validation path that does not require pre-existing store data:
// malformed JSON, empty title, invalid priority (400s), the
// unrecognized-error fallback (500, via failingRepository), and
// not-found paths (404, against an empty api_test store). Success
// paths and state-dependent error shapes are covered in
// task_handler_integration_test.go.
func TestWireContract_ErrorAndValidationShapes(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(t *testing.T) http.Handler
		do         func(t *testing.T, h http.Handler) *httptest.ResponseRecorder
		wantStatus int
		wantFields []string
	}{
		// POST /tasks
		{
			name:    "POST /tasks malformed JSON body -> 400 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRawRequest(t, h, http.MethodPost, "/tasks", "{not-json")
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks empty title -> 400 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": ""})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks invalid priority -> 400 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task", "priority": "urgent"})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name: "POST /tasks repository failure -> 500 errorResponse",
			handler: func(t *testing.T) http.Handler {
				return newFailingTestHandler()
			},
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire task"})
			},
			wantStatus: http.StatusInternalServerError,
			wantFields: wireErrorFields,
		},

		// GET /tasks/{id}
		{
			name:    "GET /tasks/{id} not found -> 404 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodGet, "/tasks/does-not-exist", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/start
		{
			name:    "POST /tasks/{id}/start not found -> 404 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/start", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/complete
		{
			name:    "POST /tasks/{id}/complete not found -> 404 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/complete", nil)
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},

		// POST /tasks/{id}/priority
		{
			name:    "POST /tasks/{id}/priority malformed JSON body -> 400 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRawRequest(t, h, http.MethodPost, "/tasks/some-id/priority", "{not-json")
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks/{id}/priority invalid priority value -> 400 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/some-id/priority", map[string]string{"priority": "urgent"})
			},
			wantStatus: http.StatusBadRequest,
			wantFields: wireErrorFields,
		},
		{
			name:    "POST /tasks/{id}/priority not found -> 404 errorResponse",
			handler: newIntegrationTestHandler,
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPost, "/tasks/does-not-exist/priority", map[string]string{"priority": "high"})
			},
			wantStatus: http.StatusNotFound,
			wantFields: wireErrorFields,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.handler(t)
			rec := tt.do(t, h)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			assertWireShape(t, decodeMap(t, rec), tt.wantFields)
		})
	}
}
