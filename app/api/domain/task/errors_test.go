package task_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// TestValidationError covers ISSUE-018's ValidationError category:
// errors.As must recognize the category type, and the wrapped
// sentinel must still satisfy errors.Is (the back-compat contract the
// plan relies on for value-object callers that predate this Issue).
func TestValidationError(t *testing.T) {
	err := task.NewValidationError("title must not be empty", task.ErrEmptyTitle)

	var ve *task.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("errors.As(err, &ValidationError{}) = false, want true (err = %v)", err)
	}
	if ve.Msg != "title must not be empty" {
		t.Errorf("Msg = %q, want %q", ve.Msg, "title must not be empty")
	}
	if !errors.Is(err, task.ErrEmptyTitle) {
		t.Errorf("errors.Is(err, ErrEmptyTitle) = false, want true (err = %v)", err)
	}
	if err.Error() == "" {
		t.Error("Error() is empty, want non-empty")
	}
}

// TestNotFoundError covers ISSUE-018's NotFoundError category, and the
// errors.Is(err, ErrNotFound) contract that
// task.DuplicateChecker.IsDuplicated depends on.
func TestNotFoundError(t *testing.T) {
	err := task.NewNotFoundError()

	var nfe *task.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("errors.As(err, &NotFoundError{}) = false, want true (err = %v)", err)
	}
	if !errors.Is(err, task.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false, want true (err = %v)", err)
	}
	if err.Error() == "" {
		t.Error("Error() is empty, want non-empty")
	}
}

// TestConflictError covers ISSUE-018's ConflictError category in both
// shapes producers use: NewDuplicateTitleError (Err set, wraps
// ErrDuplicateTitle) and a bare Msg-only ConflictError with a nil Err
// (the shape TransitionError.Unwrap produces).
func TestConflictError(t *testing.T) {
	t.Run("wraps a sentinel (duplicate title)", func(t *testing.T) {
		err := task.NewDuplicateTitleError()

		var ce *task.ConflictError
		if !errors.As(err, &ce) {
			t.Fatalf("errors.As(err, &ConflictError{}) = false, want true (err = %v)", err)
		}
		if ce.Msg != "task title already exists" {
			t.Errorf("Msg = %q, want %q", ce.Msg, "task title already exists")
		}
		if !errors.Is(err, task.ErrDuplicateTitle) {
			t.Errorf("errors.Is(err, ErrDuplicateTitle) = false, want true (err = %v)", err)
		}
		if err.Error() == "" {
			t.Error("Error() is empty, want non-empty")
		}
	})

	t.Run("built with a nil Err (the shape TransitionError.Unwrap produces)", func(t *testing.T) {
		msg := "task: cannot transition from doing to doing"
		err := task.NewConflictError(msg, nil)

		var ce *task.ConflictError
		if !errors.As(err, &ce) {
			t.Fatalf("errors.As(err, &ConflictError{}) = false, want true (err = %v)", err)
		}
		if ce.Msg != msg {
			t.Errorf("Msg = %q, want %q", ce.Msg, msg)
		}
		if err.Error() != msg {
			t.Errorf("Error() = %q, want %q (Msg-only, no wrapped Err)", err.Error(), msg)
		}
	})
}

// TestDBError covers ISSUE-018's DBError category: errors.As
// recognizes the category, the Unwrap chain reaches the inner error
// (so errors.Is(err, inner) works for callers/tests that need it), and
// -- critically -- a DBError never satisfies errors.As(&ValidationError{})
// even when its inner error's *text* looks like a validation failure.
// This mirrors infra/postgres's taskFromRow, which deliberately
// stringifies (%v, not %w) an inner *ValidationError from a corrupt
// row decode to sever it from the Unwrap chain, so a corrupt DB row
// surfaces as HTTP 500 (DBError) rather than HTTP 400
// (ValidationError).
func TestDBError(t *testing.T) {
	inner := errors.New("connection refused")
	err := task.NewDBError(inner)

	var dbe *task.DBError
	if !errors.As(err, &dbe) {
		t.Fatalf("errors.As(err, &DBError{}) = false, want true (err = %v)", err)
	}
	if !errors.Is(err, inner) {
		t.Errorf("errors.Is(err, inner) = false, want true (Unwrap chain should reach inner)")
	}
	if err.Error() == "" {
		t.Error("Error() is empty, want non-empty")
	}

	t.Run("does not chain into ValidationError when inner error is %v-severed", func(t *testing.T) {
		// Models taskFromRow's decode-failure wrapping: the inner
		// *ValidationError is embedded via %v (string formatting),
		// not %w (chaining), so it must be unreachable via errors.As.
		corruptRowErr := task.NewDBError(fmt.Errorf(
			"decode task row priority %q: %v", "bogus",
			&task.ValidationError{Msg: "invalid priority", Err: task.ErrInvalidPriority},
		))

		var ve *task.ValidationError
		if errors.As(corruptRowErr, &ve) {
			t.Errorf("errors.As(err, &ValidationError{}) = true, want false (inner ValidationError must be severed by %%v, not chained via %%w)")
		}

		var dbe2 *task.DBError
		if !errors.As(corruptRowErr, &dbe2) {
			t.Errorf("errors.As(err, &DBError{}) = false, want true")
		}
	})
}

// TestTransitionError covers ISSUE-018's requirement that
// TransitionError remains discoverable as *both* its own concrete type
// (so From/To stay inspectable, per the plan's back-compat guarantee
// for task_test.go / task_service_test.go) and as a *ConflictError (so
// route's type switch maps it to 409 without enumerating
// TransitionError separately).
func TestTransitionError(t *testing.T) {
	te := &task.TransitionError{From: task.StatusDoing, To: task.StatusDoing}
	var err error = te

	var gotTE *task.TransitionError
	if !errors.As(err, &gotTE) {
		t.Fatalf("errors.As(err, &TransitionError{}) = false, want true (err = %v)", err)
	}
	if gotTE.From != task.StatusDoing || gotTE.To != task.StatusDoing {
		t.Errorf("TransitionError = {From: %v, To: %v}, want {From: %v, To: %v}",
			gotTE.From, gotTE.To, task.StatusDoing, task.StatusDoing)
	}

	var ce *task.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("errors.As(err, &ConflictError{}) = false, want true (err = %v)", err)
	}
	if ce.Msg != te.Error() {
		t.Errorf("ConflictError.Msg = %q, want %q (te.Error())", ce.Msg, te.Error())
	}
}

// TestCategoryTypes_SatisfyErrorInterface is a cheap but explicit
// regression guard: every category type (plus TransitionError) must
// implement the error interface with a non-empty message, since
// route.writeError and callers throughout the codebase rely on
// err.Error() never being blank.
func TestCategoryTypes_SatisfyErrorInterface(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ValidationError", task.NewValidationError("msg", task.ErrEmptyTitle)},
		{"NotFoundError", task.NewNotFoundError()},
		{"ConflictError", task.NewConflictError("msg", task.ErrDuplicateTitle)},
		{"DBError", task.NewDBError(errors.New("boom"))},
		{"TransitionError", &task.TransitionError{From: task.StatusTodo, To: task.StatusDone}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() == "" {
				t.Error("Error() is empty, want non-empty")
			}
		})
	}
}
