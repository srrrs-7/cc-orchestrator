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

	tk := task.New(title, task.PriorityMedium)

	if tk.Status() != task.StatusTodo {
		t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusTodo)
	}
	if tk.Title() != title {
		t.Errorf("Title() = %v, want %v", tk.Title(), title)
	}
	// R1: New stores the priority it is given.
	if tk.Priority() != task.PriorityMedium {
		t.Errorf("Priority() = %v, want %v", tk.Priority(), task.PriorityMedium)
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

// TestNew_Priority covers R1 (the aggregate holds whatever Priority it
// is constructed with, for every known priority value).
func TestNew_Priority(t *testing.T) {
	tests := []struct {
		name     string
		priority task.Priority
	}{
		{name: "low", priority: task.PriorityLow},
		{name: "medium", priority: task.PriorityMedium},
		{name: "high", priority: task.PriorityHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := task.New(newTestTitle(t, "buy milk"), tt.priority)

			if tk.Priority() != tt.priority {
				t.Errorf("Priority() = %v, want %v", tk.Priority(), tt.priority)
			}
		})
	}
}

func TestTask_Start(t *testing.T) {
	title := newTestTitle(t, "buy milk")

	t.Run("todo to doing succeeds", func(t *testing.T) {
		tk := task.New(title, task.PriorityMedium)

		if err := tk.Start(); err != nil {
			t.Fatalf("Start() unexpected error: %v", err)
		}
		if tk.Status() != task.StatusDoing {
			t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusDoing)
		}
	})

	t.Run("doing to doing fails with TransitionError", func(t *testing.T) {
		tk := task.New(title, task.PriorityMedium)
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
		tk := task.New(title, task.PriorityMedium)
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
		tk := task.New(title, task.PriorityMedium)

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
		tk := task.New(title, task.PriorityMedium)
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

// TestTask_PriorityOrthogonalToTransitions is the non-functional
// requirement from the plan: priority must not interfere with the
// todo->doing->done state machine in either direction. A task created
// with the highest priority must transition exactly like any other,
// and invalid transitions must still fail the same way.
func TestTask_PriorityOrthogonalToTransitions(t *testing.T) {
	tk := task.New(newTestTitle(t, "buy milk"), task.PriorityHigh)

	if err := tk.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	if tk.Status() != task.StatusDoing {
		t.Errorf("Status() after Start() = %v, want %v", tk.Status(), task.StatusDoing)
	}
	if tk.Priority() != task.PriorityHigh {
		t.Errorf("Priority() after Start() = %v, want unchanged %v", tk.Priority(), task.PriorityHigh)
	}

	if err := tk.Complete(); err != nil {
		t.Fatalf("Complete() unexpected error: %v", err)
	}
	if tk.Status() != task.StatusDone {
		t.Errorf("Status() after Complete() = %v, want %v", tk.Status(), task.StatusDone)
	}
	if tk.Priority() != task.PriorityHigh {
		t.Errorf("Priority() after Complete() = %v, want unchanged %v", tk.Priority(), task.PriorityHigh)
	}

	// An invalid transition (done -> done) must still fail exactly as
	// it does for any other priority.
	err := tk.Complete()
	var transitionErr *task.TransitionError
	if !errors.As(err, &transitionErr) {
		t.Fatalf("Complete() error = %v, want *task.TransitionError", err)
	}
}

func TestReconstruct(t *testing.T) {
	title := newTestTitle(t, "buy milk")
	id, err := task.ParseID("fixed-id")
	if err != nil {
		t.Fatalf("ParseID() unexpected error: %v", err)
	}
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	tk := task.Reconstruct(id, title, task.StatusDoing, task.PriorityHigh, createdAt, updatedAt)

	if tk.ID() != id {
		t.Errorf("ID() = %v, want %v", tk.ID(), id)
	}
	if tk.Title() != title {
		t.Errorf("Title() = %v, want %v", tk.Title(), title)
	}
	if tk.Status() != task.StatusDoing {
		t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusDoing)
	}
	// R1: Reconstruct round-trips the priority it is given, unchanged.
	if tk.Priority() != task.PriorityHigh {
		t.Errorf("Priority() = %v, want %v", tk.Priority(), task.PriorityHigh)
	}
	if !tk.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt() = %v, want %v", tk.CreatedAt(), createdAt)
	}
	if !tk.UpdatedAt().Equal(updatedAt) {
		t.Errorf("UpdatedAt() = %v, want %v", tk.UpdatedAt(), updatedAt)
	}
}

func TestTask_Rename(t *testing.T) {
	tk := task.New(newTestTitle(t, "buy milk"), task.PriorityMedium)
	newTitle := newTestTitle(t, "buy oat milk")

	tk.Rename(newTitle)

	if tk.Title() != newTitle {
		t.Errorf("Title() after Rename() = %v, want %v", tk.Title(), newTitle)
	}
}

// TestTask_ChangePriority covers R3: ChangePriority replaces the
// priority, bumps updatedAt, and — critically — leaves status
// untouched (priority changes are orthogonal to the todo/doing/done
// state machine).
func TestTask_ChangePriority(t *testing.T) {
	t.Run("changes priority and updatedAt without touching status", func(t *testing.T) {
		tk := task.New(newTestTitle(t, "buy milk"), task.PriorityLow)
		if err := tk.Start(); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}
		before := tk.UpdatedAt()

		tk.ChangePriority(task.PriorityHigh)

		if tk.Priority() != task.PriorityHigh {
			t.Errorf("Priority() = %v, want %v", tk.Priority(), task.PriorityHigh)
		}
		if tk.Status() != task.StatusDoing {
			t.Errorf("Status() after ChangePriority() = %v, want unchanged %v", tk.Status(), task.StatusDoing)
		}
		// Not a strict After(): two back-to-back time.Now() calls
		// (Start() then ChangePriority()) can observe the same
		// instant at typical clock resolutions, which made this
		// assertion flaky (about 1/4 of runs). ChangePriority's
		// contract is "never moves updatedAt backwards", not
		// "always strictly increases it within the same tick", so
		// assert non-regression (!Before, i.e. >=) instead. See
		// .claude/rules/testing.md: tests must not depend on real
		// time precision.
		if tk.UpdatedAt().Before(before) {
			t.Errorf("UpdatedAt() = %v, want not before %v", tk.UpdatedAt(), before)
		}
	})

	t.Run("priority change does not block subsequent transitions", func(t *testing.T) {
		tk := task.New(newTestTitle(t, "buy milk"), task.PriorityMedium)
		tk.ChangePriority(task.PriorityLow)

		if err := tk.Start(); err != nil {
			t.Fatalf("Start() unexpected error: %v", err)
		}
		if tk.Status() != task.StatusDoing {
			t.Errorf("Status() = %v, want %v", tk.Status(), task.StatusDoing)
		}
	})
}
