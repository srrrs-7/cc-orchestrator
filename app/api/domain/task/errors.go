package task

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by the task domain package. Callers should
// use errors.Is to branch on these, since they may be wrapped with
// additional context via fmt.Errorf("...: %w", err).
var (
	// ErrNotFound is returned by Repository lookups when no matching
	// Task exists.
	ErrNotFound = errors.New("task: not found")

	// ErrDuplicateTitle is returned when a Task with the same Title
	// already exists.
	ErrDuplicateTitle = errors.New("task: duplicate title")

	// ErrEmptyTitle is returned when a Title is empty after trimming.
	ErrEmptyTitle = errors.New("task: title is empty")

	// ErrTitleTooLong is returned when a Title exceeds the maximum
	// allowed length.
	ErrTitleTooLong = errors.New("task: title is too long")

	// ErrInvalidID is returned when an ID cannot be parsed from a
	// string (e.g. it is empty).
	ErrInvalidID = errors.New("task: invalid id")

	// ErrInvalidStatus is returned when a Status cannot be parsed
	// from a string.
	ErrInvalidStatus = errors.New("task: invalid status")
)

// TransitionError indicates an attempt to move a Task from one Status
// to another that violates the allowed state machine. It is a custom
// error type (rather than a sentinel) because it carries the From/To
// values, allowing callers to inspect the failed transition via
// errors.As.
type TransitionError struct {
	From Status
	To   Status
}

// Error implements the error interface.
func (e *TransitionError) Error() string {
	return fmt.Sprintf("task: cannot transition from %s to %s", e.From, e.To)
}
