package route_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/api/route"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// taskResponseBody is the typed decoder for single-task responses.
// Defined here (integration-only) because it is not used in the
// untagged offline tests.
type taskResponseBody struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func decodeTask(t *testing.T, rec *httptest.ResponseRecorder) taskResponseBody {
	t.Helper()
	var got taskResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

// wireTaskFields is the exact, snake_case field set of route.taskResponse.
// Defined here (integration-only) because it is not used in the untagged
// offline tests.
var wireTaskFields = []string{"id", "title", "status", "priority", "created_at", "updated_at"}

// newIntegrationTestHandler opens a real Postgres connection via
// testsupport.OpenTestDB (against the dedicated api_test database;
// see testsupport.OpenTestDB's REQUIRE_DB fail-closed policy), truncates
// the tasks table, and returns an http.Handler backed by
// postgres.NewTaskRepository. Each call truncates so every test case
// starts from an empty store. The DB is closed by t.Cleanup. It is
// shared by both this file's success/state-dependent suite and
// task_handler_test.go's validation/not-found tests (SPEC-013: this
// package no longer splits into an untagged half and a
// `//go:build integration` half -- everything here runs together as
// part of the default `make test`).
func newIntegrationTestHandler(t *testing.T) http.Handler {
	t.Helper()
	db := testsupport.OpenTestDB(t)
	testsupport.TruncateTasks(t, db)
	repo := postgres.NewTaskRepository(db)
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, repo, dupChk)
	return route.NewRouter(svc)
}

// --- happy-path / state-dependent tests --------------------------------

func TestCreateTask_Success(t *testing.T) {
	h := newIntegrationTestHandler(t)

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
			h := newIntegrationTestHandler(t)

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

// TestCreateTask_ExplicitNullPriority covers the boundary flagged in
// the plan's risk section: an explicitly-sent JSON null for priority
// (as opposed to an omitted field or an explicit "") must decode to
// Go's zero value for string ("") without a decode error, and
// therefore also default to medium (R2), exactly like an omitted or
// empty priority.
func TestCreateTask_ExplicitNullPriority(t *testing.T) {
	h := newIntegrationTestHandler(t)

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
	h := newIntegrationTestHandler(t)

	first := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"})
	if first.Code != http.StatusCreated {
		t.Fatalf("setup: status = %d, want %d (body=%q)", first.Code, http.StatusCreated, first.Body.String())
	}

	rec := doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestGetTask_Success(t *testing.T) {
	h := newIntegrationTestHandler(t)

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
	h := newIntegrationTestHandler(t)

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
	h := newIntegrationTestHandler(t)

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
	h := newIntegrationTestHandler(t)

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

// TestListTasks_LimitClampedAboveMax covers R3: a limit above
// task.MaxLimit (100) is clamped rather than rejected -- the request
// still succeeds (200), and the envelope's echoed limit reflects the
// clamp (100), not the raw requested value (1000).
func TestListTasks_LimitClampedAboveMax(t *testing.T) {
	h := newIntegrationTestHandler(t)
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
	h := newIntegrationTestHandler(t)
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
	h := newIntegrationTestHandler(t)
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
	h := newIntegrationTestHandler(t)

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

// ---------------------------------------------------------------------
// Wire-contract tests (SPEC-003 T2 / R2) -- success/state-dependent half.
//
// This half covers success paths and state-dependent error shapes
// (duplicate, invalid transition) against the real api_test database.
// Error/validation paths that do not need pre-existing store state are
// in task_handler_test.go's TestWireContract_ErrorAndValidationShapes,
// which shares this file's doRequest/decodeMap/assertWireShape
// helpers -- both halves live in the same untagged route_test package
// and run together as part of the default `make test` (SPEC-013).
// ---------------------------------------------------------------------

// wireListEnvelopeFields is the exact field set of
// route.taskListResponse (SPEC-008's envelope), excluding "items"
// itself (its element shape is pinned separately via wireTaskFields).
var wireListEnvelopeFields = []string{"items", "total", "limit", "offset"}

// TestWireContract_SuccessAndStateShapes covers every Task-returning
// endpoint (create/get/start/complete/changePriority) and state-
// dependent error modes (duplicate title, invalid transition), pinning:
//   - success bodies are exactly {id,title,status,priority,
//     created_at,updated_at}, snake_case, all-string (taskResponse);
//   - failure bodies are exactly {error} (errorResponse);
//   - the status code produced for each scenario.
func TestWireContract_SuccessAndStateShapes(t *testing.T) {
	tests := []struct {
		name       string
		do         func(t *testing.T, h http.Handler) *httptest.ResponseRecorder
		wantStatus int
		wantFields []string
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
			name: "POST /tasks duplicate title -> 409 errorResponse",
			do: func(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
				doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire dup"})
				return doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "wire dup"})
			},
			wantStatus: http.StatusConflict,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newIntegrationTestHandler(t)
			rec := tt.do(t, h)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			assertWireShape(t, decodeMap(t, rec), tt.wantFields)
		})
	}
}

// TestWireContract_ListResponseFields pins GET /tasks's shape
// (SPEC-008): a JSON envelope object {items,total,limit,offset}
// whose every items[] element is a taskResponse (the same exact
// field set and casing pinned above for the single-object
// endpoints).
func TestWireContract_ListResponseFields(t *testing.T) {
	h := newIntegrationTestHandler(t)
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
