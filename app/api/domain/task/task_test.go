package task_test

import (
	"errors"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

func newTestTitle(t *testing.T, s string) task.Title {
	t.Helper()
	title, err := task.NewTitle(s)
	if err != nil {
		t.Fatalf("NewTitle(%q) unexpected error: %v", s, err)
	}
	return title
}

func TestNew_InitialState(t *testing.T) {
	title := newTestTitle(t, "buy milk")

	tk := task.New(title)

	if tk.Status() != task.StatusTodo {
		t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusTodo)
	}
	if tk.Title() != title {
		t.Errorf("Title() = %v, want %v", tk.Title(), title)
	}
	if tk.ID().String() == "" {
		t.Error("ID().String() is empty, want non-empty")
	}
	if tk.CreatedAt().IsZero() {
		t.Error("CreatedAt() is zero, want set")
	}
	if tk.UpdatedAt().IsZero() {
		t.Error("UpdatedAt() is zero, want set")
	}
}

func TestTask_Start(t *testing.T) {
	title := newTestTitle(t, "buy milk")

	t.Run("todo to doing succeeds", func(t *testing.T) {
		tk := task.New(title)

		if err := tk.Start(); err != nil {
			t.Fatalf("Start() unexpected error: %v", err)
		}
		if tk.Status() != task.StatusDoing {
			t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusDoing)
		}
	})

	t.Run("doing to doing fails with TransitionError", func(t *testing.T) {
		tk := task.New(title)
		if err := tk.Start(); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}

		err := tk.Start()
		if err == nil {
			t.Fatal("Start() error = nil, want *task.TransitionError")
		}

		var transitionErr *task.TransitionError
		if !errors.As(err, &transitionErr) {
			t.Fatalf("errors.As() = false, want true (err = %v)", err)
		}
		if transitionErr.From != task.StatusDoing || transitionErr.To != task.StatusDoing {
			t.Errorf("TransitionError = {From: %v, To: %v}, want {From: %v, To: %v}",
				transitionErr.From, transitionErr.To, task.StatusDoing, task.StatusDoing)
		}
		if tk.Status() != task.StatusDoing {
			t.Errorf("Status() after failed transition = %v, want unchanged %v", tk.Status(), task.StatusDoing)
		}
	})
}

func TestTask_Complete(t *testing.T) {
	title := newTestTitle(t, "buy milk")

	t.Run("doing to done succeeds", func(t *testing.T) {
		tk := task.New(title)
		if err := tk.Start(); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}

		if err := tk.Complete(); err != nil {
			t.Fatalf("Complete() unexpected error: %v", err)
		}
		if tk.Status() != task.StatusDone {
			t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusDone)
		}
	})

	t.Run("todo to done fails with TransitionError", func(t *testing.T) {
		tk := task.New(title)

		err := tk.Complete()

		var transitionErr *task.TransitionError
		if !errors.As(err, &transitionErr) {
			t.Fatalf("errors.As() = false, want true (err = %v)", err)
		}
		if transitionErr.From != task.StatusTodo || transitionErr.To != task.StatusDone {
			t.Errorf("TransitionError = {From: %v, To: %v}, want {From: %v, To: %v}",
				transitionErr.From, transitionErr.To, task.StatusTodo, task.StatusDone)
		}
		if tk.Status() != task.StatusTodo {
			t.Errorf("Status() after failed transition = %v, want unchanged %v", tk.Status(), task.StatusTodo)
		}
	})

	t.Run("done to done fails with TransitionError", func(t *testing.T) {
		tk := task.New(title)
		if err := tk.Start(); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}
		if err := tk.Complete(); err != nil {
			t.Fatalf("setup Complete() unexpected error: %v", err)
		}

		err := tk.Complete()

		var transitionErr *task.TransitionError
		if !errors.As(err, &transitionErr) {
			t.Fatalf("errors.As() = false, want true (err = %v)", err)
		}
		if transitionErr.From != task.StatusDone || transitionErr.To != task.StatusDone {
			t.Errorf("TransitionError = {From: %v, To: %v}, want {From: %v, To: %v}",
				transitionErr.From, transitionErr.To, task.StatusDone, task.StatusDone)
		}
	})
}

func TestReconstruct(t *testing.T) {
	title := newTestTitle(t, "buy milk")
	id, err := task.ParseID("fixed-id")
	if err != nil {
		t.Fatalf("ParseID() unexpected error: %v", err)
	}
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	tk := task.Reconstruct(id, title, task.StatusDoing, createdAt, updatedAt)

	if tk.ID() != id {
		t.Errorf("ID() = %v, want %v", tk.ID(), id)
	}
	if tk.Title() != title {
		t.Errorf("Title() = %v, want %v", tk.Title(), title)
	}
	if tk.Status() != task.StatusDoing {
		t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusDoing)
	}
	if !tk.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt() = %v, want %v", tk.CreatedAt(), createdAt)
	}
	if !tk.UpdatedAt().Equal(updatedAt) {
		t.Errorf("UpdatedAt() = %v, want %v", tk.UpdatedAt(), updatedAt)
	}
}

func TestTask_Rename(t *testing.T) {
	tk := task.New(newTestTitle(t, "buy milk"))
	newTitle := newTestTitle(t, "buy oat milk")

	tk.Rename(newTitle)

	if tk.Title() != newTitle {
		t.Errorf("Title() after Rename() = %v, want %v", tk.Title(), newTitle)
	}
}
