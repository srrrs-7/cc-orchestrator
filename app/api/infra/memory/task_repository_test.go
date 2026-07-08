package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/memory"
)

func newTestTask(t *testing.T, title string) *task.Task {
	t.Helper()
	tt, err := task.NewTitle(title)
	if err != nil {
		t.Fatalf("NewTitle(%q) unexpected error: %v", title, err)
	}
	return task.New(tt, task.PriorityMedium)
}

func TestTaskRepository_SaveAndFindByID(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")

	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got.ID() != tk.ID() {
		t.Errorf("ID() = %v, want %v", got.ID(), tk.ID())
	}
	if got.Title() != tk.Title() {
		t.Errorf("Title() = %v, want %v", got.Title(), tk.Title())
	}
	if got.Status() != tk.Status() {
		t.Errorf("Status() = %v, want %v", got.Status(), tk.Status())
	}
	// R1: clone (used by Save/FindByID under the hood) preserves priority.
	if got.Priority() != tk.Priority() {
		t.Errorf("Priority() = %v, want %v", got.Priority(), tk.Priority())
	}
}

// TestTaskRepository_SaveAndFindByID_PreservesPriority is a dedicated
// R1 regression: a non-default priority set at construction time must
// survive the Save -> FindByID round trip through clone()/Reconstruct.
func TestTaskRepository_SaveAndFindByID_PreservesPriority(t *testing.T) {
	repo := memory.NewTaskRepository()
	title, err := task.NewTitle("buy milk")
	if err != nil {
		t.Fatalf("NewTitle() unexpected error: %v", err)
	}
	tk := task.New(title, task.PriorityHigh)

	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got.Priority() != task.PriorityHigh {
		t.Errorf("Priority() = %v, want %v", got.Priority(), task.PriorityHigh)
	}
}

func TestTaskRepository_FindByID_NotFound(t *testing.T) {
	repo := memory.NewTaskRepository()

	_, err := repo.FindByID(context.Background(), task.NewID())
	if !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("FindByID() error = %v, want wrapping %v", err, task.ErrNotFound)
	}
}

func TestTaskRepository_FindByTitle(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")
	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	t.Run("existing title is found", func(t *testing.T) {
		got, err := repo.FindByTitle(context.Background(), tk.Title())
		if err != nil {
			t.Fatalf("FindByTitle() unexpected error: %v", err)
		}
		if got.ID() != tk.ID() {
			t.Errorf("ID() = %v, want %v", got.ID(), tk.ID())
		}
	})

	t.Run("unknown title is not found", func(t *testing.T) {
		other, err := task.NewTitle("walk dog")
		if err != nil {
			t.Fatalf("NewTitle() unexpected error: %v", err)
		}

		_, err = repo.FindByTitle(context.Background(), other)
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("FindByTitle() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

func TestTaskRepository_FindAll(t *testing.T) {
	repo := memory.NewTaskRepository()

	all, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll() unexpected error: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("FindAll() on empty repo = %d items, want 0", len(all))
	}

	tk1 := newTestTask(t, "buy milk")
	tk2 := newTestTask(t, "walk dog")
	if err := repo.Save(context.Background(), tk1); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}
	if err := repo.Save(context.Background(), tk2); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	all, err = repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("FindAll() unexpected error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("FindAll() = %d items, want 2", len(all))
	}

	ids := map[task.ID]bool{}
	for _, tk := range all {
		ids[tk.ID()] = true
	}
	if !ids[tk1.ID()] || !ids[tk2.ID()] {
		t.Errorf("FindAll() = %v, want to contain both %v and %v", ids, tk1.ID(), tk2.ID())
	}
}

// TestTaskRepository_Save_DoesNotAliasCallersTask verifies that Save
// stores a clone: mutating the caller's *task.Task after Save must
// not affect the repository's stored state.
func TestTaskRepository_Save_DoesNotAliasCallersTask(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")

	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	// Mutate the caller's task after it has been saved.
	if err := tk.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got.Status() != task.StatusTodo {
		t.Errorf("stored Status() = %v, want %v (mutation of caller's task leaked into repository)", got.Status(), task.StatusTodo)
	}
}

// TestTaskRepository_FindByID_ReturnsIndependentCopies verifies that
// the *task.Task returned by FindByID is a clone: mutating it must
// not affect the repository's stored state or subsequent reads.
func TestTaskRepository_FindByID_ReturnsIndependentCopies(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")
	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got1, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if err := got1.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}

	got2, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got2.Status() != task.StatusTodo {
		t.Errorf("stored Status() = %v, want %v (mutation of returned task leaked into repository)", got2.Status(), task.StatusTodo)
	}
}
