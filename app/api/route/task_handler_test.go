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
	if got.ID == "" {
		t.Error("ID is empty, want non-empty")
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

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"}))

	rec := doRequest(t, h, http.MethodGet, "/tasks/"+created.ID, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decodeTask(t, rec)
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
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

	created := decodeTask(t, doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"}))

	startRec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/start", nil)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start: status = %d, want %d (body=%q)", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	if got := decodeTask(t, startRec); got.Status != "doing" {
		t.Errorf("start: Status = %q, want %q", got.Status, "doing")
	}

	completeRec := doRequest(t, h, http.MethodPost, "/tasks/"+created.ID+"/complete", nil)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("complete: status = %d, want %d (body=%q)", completeRec.Code, http.StatusOK, completeRec.Body.String())
	}
	if got := decodeTask(t, completeRec); got.Status != "done" {
		t.Errorf("complete: Status = %q, want %q", got.Status, "done")
	}
}

func TestListTasks(t *testing.T) {
	h := newTestHandler()

	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "buy milk"})
	doRequest(t, h, http.MethodPost, "/tasks", map[string]string{"title": "walk dog"})

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
}
