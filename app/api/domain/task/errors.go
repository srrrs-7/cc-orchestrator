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

	// ErrInvalidPriority is returned when a Priority cannot be parsed
	// from a string.
	ErrInvalidPriority = errors.New("task: invalid priority")
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

// Unwrap exposes the TransitionError as a *ConflictError, so that
// callers (namely route.writeError) can branch on the category type
// without needing to enumerate TransitionError separately.
func (e *TransitionError) Unwrap() error {
	return &ConflictError{Msg: e.Error()}
}

// ValidationError indicates that caller-supplied input failed a
// domain validation rule (e.g. an empty title, an unparsable
// priority). It maps to HTTP 400 Bad Request in the route layer.
type ValidationError struct {
	Msg string
	Err error
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s", e.Msg, e.Err)
	}
	return e.Msg
}

// Unwrap exposes the wrapped sentinel error, so that existing
// errors.Is(err, ErrXxx) assertions continue to work.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

// NewValidationError builds a *ValidationError wrapping err with msg.
func NewValidationError(msg string, err error) *ValidationError {
	return &ValidationError{Msg: msg, Err: err}
}

// NotFoundError indicates that a lookup found no matching Task. It
// maps to HTTP 404 Not Found in the route layer.
type NotFoundError struct{}

// Error implements the error interface.
func (e *NotFoundError) Error() string {
	return ErrNotFound.Error()
}

// Unwrap exposes ErrNotFound, so that existing errors.Is(err,
// ErrNotFound) assertions (e.g. DuplicateChecker.IsDuplicated)
// continue to work.
func (e *NotFoundError) Unwrap() error {
	return ErrNotFound
}

// NewNotFoundError builds a *NotFoundError.
func NewNotFoundError() *NotFoundError {
	return &NotFoundError{}
}

// ConflictError indicates that a request conflicts with the current
// state of a Task (e.g. a duplicate title, an invalid status
// transition). It maps to HTTP 409 Conflict in the route layer.
type ConflictError struct {
	Msg string
	Err error
}

// Error implements the error interface.
func (e *ConflictError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s", e.Msg, e.Err)
	}
	return e.Msg
}

// Unwrap exposes the wrapped sentinel error (which may be nil), so
// that existing errors.Is(err, ErrXxx) assertions continue to work.
func (e *ConflictError) Unwrap() error {
	return e.Err
}

// NewConflictError builds a *ConflictError wrapping err with msg.
func NewConflictError(msg string, err error) *ConflictError {
	return &ConflictError{Msg: msg, Err: err}
}

// NewDuplicateTitleError builds the *ConflictError returned when a
// Task with the same Title already exists.
func NewDuplicateTitleError() *ConflictError {
	return &ConflictError{Msg: "task title already exists", Err: ErrDuplicateTitle}
}

// DBError indicates an infrastructure-layer failure (e.g. a database
// or context error) that is not meaningful to the caller. It maps to
// HTTP 500 Internal Server Error in the route layer; the underlying
// Err is logged but never exposed in the response body.
type DBError struct {
	Err error
}

// Error implements the error interface.
func (e *DBError) Error() string {
	return fmt.Sprintf("task: database error: %s", e.Err)
}

// Unwrap exposes the wrapped underlying error.
func (e *DBError) Unwrap() error {
	return e.Err
}

// NewDBError builds a *DBError wrapping err.
func NewDBError(err error) *DBError {
	return &DBError{Err: err}
}
