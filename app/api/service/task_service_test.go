package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// fakeRepository is an in-memory task.Repository fake used to
// exercise TaskService without depending on the infra layer.
type fakeRepository struct {
	mu    sync.Mutex
	tasks map[task.ID]*task.Task
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{tasks: make(map[task.ID]*task.Task)}
}

func (f *fakeRepository) Save(ctx context.Context, t *task.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks[t.ID()] = t
	return nil
}

func (f *fakeRepository) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, task.ErrNotFound
	}
	return t, nil
}

func (f *fakeRepository) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tasks {
		if t.Title() == title {
			return t, nil
		}
	}
	return nil, task.ErrNotFound
}

func (f *fakeRepository) FindAll(ctx context.Context) ([]*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]*task.Task, 0, len(f.tasks))
	for _, t := range f.tasks {
		result = append(result, t)
	}
	return result, nil
}

func newTestService() (*service.TaskService, *fakeRepository) {
	repo := newFakeRepository()
	dupChk := task.NewDuplicateChecker(repo)
	return service.NewTaskService(repo, dupChk), repo
}

func TestTaskService_Create_Success(t *testing.T) {
	svc, _ := newTestService()

	dto, err := svc.Create(context.Background(), "buy milk", "")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if dto.Title != "buy milk" {
		t.Errorf("Title = %q, want %q", dto.Title, "buy milk")
	}
	if dto.Status != task.StatusTodo.String() {
		t.Errorf("Status = %q, want %q", dto.Status, task.StatusTodo.String())
	}
	// R2: an unspecified (empty) priority defaults to medium.
	if dto.Priority != task.PriorityMedium.String() {
		t.Errorf("Priority = %q, want %q", dto.Priority, task.PriorityMedium.String())
	}
	if dto.ID == "" {
		t.Error("ID is empty, want non-empty")
	}
	if dto.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want set")
	}
	if dto.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want set")
	}
}

// TestTaskService_Create_Priority covers R2 (an unspecified/empty
// priority defaults to medium; an explicit priority is honored
// verbatim) and R5 (an invalid priority string is rejected with
// ErrInvalidPriority instead of silently defaulting).
func TestTaskService_Create_Priority(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		want     string
		wantErr  error
	}{
		{name: "unspecified defaults to medium (R2 boundary)", priority: "", want: task.PriorityMedium.String()},
		{name: "explicit low", priority: "low", want: task.PriorityLow.String()},
		{name: "explicit medium", priority: "medium", want: task.PriorityMedium.String()},
		{name: "explicit high", priority: "high", want: task.PriorityHigh.String()},
		{name: "invalid priority is rejected (R5)", priority: "urgent", wantErr: task.ErrInvalidPriority},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestService()

			dto, err := svc.Create(context.Background(), "buy milk", tt.priority)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create(_, _, %q) error = %v, want wrapping %v", tt.priority, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create(_, _, %q) unexpected error: %v", tt.priority, err)
			}
			if dto.Priority != tt.want {
				t.Errorf("Priority = %q, want %q", dto.Priority, tt.want)
			}
		})
	}
}

func TestTaskService_Create_DuplicateTitle(t *testing.T) {
	svc, _ := newTestService()

	if _, err := svc.Create(context.Background(), "buy milk", ""); err != nil {
		t.Fatalf("setup Create() unexpected error: %v", err)
	}

	_, err := svc.Create(context.Background(), "buy milk", "")
	if !errors.Is(err, task.ErrDuplicateTitle) {
		t.Fatalf("Create() error = %v, want wrapping %v", err, task.ErrDuplicateTitle)
	}
}

func TestTaskService_Create_InvalidTitle(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		wantErr error
	}{
		{name: "empty title", title: "", wantErr: task.ErrEmptyTitle},
		{name: "whitespace only title", title: "   ", wantErr: task.ErrEmptyTitle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestService()

			_, err := svc.Create(context.Background(), tt.title, "")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create(%q) error = %v, want wrapping %v", tt.title, err, tt.wantErr)
			}
		})
	}
}

func TestTaskService_Get(t *testing.T) {
	t.Run("existing task is found", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		got, err := svc.Get(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		if got != created {
			t.Errorf("Get() = %+v, want %+v", got, created)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.Get(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Get() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("empty id is invalid", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.Get(context.Background(), "")
		if !errors.Is(err, task.ErrInvalidID) {
			t.Fatalf("Get() error = %v, want wrapping %v", err, task.ErrInvalidID)
		}
	})
}

func TestTaskService_List(t *testing.T) {
	svc, _ := newTestService()

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() on empty repo = %d items, want 0", len(got))
	}

	if _, err := svc.Create(context.Background(), "buy milk", ""); err != nil {
		t.Fatalf("setup Create() unexpected error: %v", err)
	}
	if _, err := svc.Create(context.Background(), "walk dog", ""); err != nil {
		t.Fatalf("setup Create() unexpected error: %v", err)
	}

	got, err = svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() = %d items, want 2", len(got))
	}
}

func TestTaskService_Start(t *testing.T) {
	t.Run("todo transitions to doing", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		got, err := svc.Start(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("Start() unexpected error: %v", err)
		}
		if got.Status != task.StatusDoing.String() {
			t.Errorf("Status = %q, want %q", got.Status, task.StatusDoing.String())
		}
	})

	t.Run("invalid transition error propagates", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}
		if _, err := svc.Start(context.Background(), created.ID); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}

		_, err = svc.Start(context.Background(), created.ID)

		var transitionErr *task.TransitionError
		if !errors.As(err, &transitionErr) {
			t.Fatalf("Start() error = %v, want *task.TransitionError", err)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.Start(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Start() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

func TestTaskService_Complete(t *testing.T) {
	t.Run("doing transitions to done", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}
		if _, err := svc.Start(context.Background(), created.ID); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}

		got, err := svc.Complete(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("Complete() unexpected error: %v", err)
		}
		if got.Status != task.StatusDone.String() {
			t.Errorf("Status = %q, want %q", got.Status, task.StatusDone.String())
		}
	})

	t.Run("invalid transition from todo propagates", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		_, err = svc.Complete(context.Background(), created.ID)

		var transitionErr *task.TransitionError
		if !errors.As(err, &transitionErr) {
			t.Fatalf("Complete() error = %v, want *task.TransitionError", err)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.Complete(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Complete() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

// TestTaskService_ChangePriority covers R3 (a valid priority change
// is persisted and reflected in the returned DTO, without touching
// status) and R5 (an invalid priority value is rejected) plus the
// not-found boundary shared with Start/Complete.
func TestTaskService_ChangePriority(t *testing.T) {
	t.Run("changes priority without touching status", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "low")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		got, err := svc.ChangePriority(context.Background(), created.ID, "high")
		if err != nil {
			t.Fatalf("ChangePriority() unexpected error: %v", err)
		}
		if got.Priority != task.PriorityHigh.String() {
			t.Errorf("Priority = %q, want %q", got.Priority, task.PriorityHigh.String())
		}
		if got.Status != task.StatusTodo.String() {
			t.Errorf("Status = %q, want unchanged %q", got.Status, task.StatusTodo.String())
		}
	})

	t.Run("invalid priority value is rejected (R5)", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		_, err = svc.ChangePriority(context.Background(), created.ID, "urgent")
		if !errors.Is(err, task.ErrInvalidPriority) {
			t.Fatalf("ChangePriority() error = %v, want wrapping %v", err, task.ErrInvalidPriority)
		}
	})

	t.Run("empty priority value is rejected (R5, strict boundary)", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		_, err = svc.ChangePriority(context.Background(), created.ID, "")
		if !errors.Is(err, task.ErrInvalidPriority) {
			t.Fatalf("ChangePriority() error = %v, want wrapping %v", err, task.ErrInvalidPriority)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.ChangePriority(context.Background(), task.NewID().String(), "high")
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("ChangePriority() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}
