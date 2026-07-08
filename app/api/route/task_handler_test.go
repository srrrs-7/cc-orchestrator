package route_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestListTasks(t *testing.T) {
	h := newTestHandler()

	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk", "priority": "low"})
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "walk dog", "priority": "high"})

	rec := doRequest(t, h, http.MethodGet, "/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []taskResponseBody
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}

	// R4: every item in the list response carries a non-empty
	// snake_case priority field.
	for _, item := range got {
		if item.Priority == "" {
			t.Errorf("item %q: Priority is empty, want set", item.Title)
		}
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
