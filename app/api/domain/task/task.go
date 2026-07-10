// Package task contains the Task aggregate and its supporting domain
// types (value objects, domain services, repository interface and
// domain errors). This package has no dependency on any other layer.
package task

import "time"

// Task is the aggregate root of the task management bounded context.
// All fields are unexported; state can only be observed via getters
// and mutated via behavioral methods, which is how the aggregate
// protects its own invariants (e.g. valid status transitions).
type Task struct {
	id        ID
	title     Title
	status    Status
	priority  Priority
	createdAt time.Time
	updatedAt time.Time
}

// New is the factory for creating a brand new Task. A new Task
// always starts in StatusTodo, and both createdAt and updatedAt are
// set to the current time. priority must already be a validated
// Priority; defaulting an unspecified priority to PriorityMedium is
// the responsibility of the application layer, not this factory.
func New(title Title, priority Priority) *Task {
	now := time.Now()
	return &Task{
		id:        NewID(),
		title:     title,
		status:    StatusTodo,
		priority:  priority,
		createdAt: now,
		updatedAt: now,
	}
}

// Reconstruct rebuilds a Task from already-validated persisted state.
// It is intended to be used exclusively by infrastructure-layer
// repository implementations when loading a Task from storage; it
// bypasses the New factory's "always starts as todo" invariant
// because the caller supplies an already-valid state snapshot.
func Reconstruct(id ID, title Title, status Status, priority Priority, createdAt, updatedAt time.Time) *Task {
	return &Task{
		id:        id,
		title:     title,
		status:    status,
		priority:  priority,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}
}

// Start transitions the Task from todo to doing. It returns a
// *TransitionError if the current status does not allow that
// transition.
func (t *Task) Start() error {
	return t.transitionTo(StatusDoing)
}

// Complete transitions the Task from doing to done. It returns a
// *TransitionError if the current status does not allow that
// transition.
func (t *Task) Complete() error {
	return t.transitionTo(StatusDone)
}

func (t *Task) transitionTo(next Status) error {
	if !t.status.CanTransitionTo(next) {
		return &TransitionError{From: t.status, To: next}
	}
	t.status = next
	t.updatedAt = time.Now()
	return nil
}

// Rename changes the Task's title.
func (t *Task) Rename(title Title) {
	t.title = title
	t.updatedAt = time.Now()
}

// ChangePriority replaces the Task's priority. It is orthogonal to
// the status state machine: it never returns an error and never
// touches status, since validation of the priority value already
// happened at construction time (via ParsePriority).
func (t *Task) ChangePriority(priority Priority) {
	t.priority = priority
	t.updatedAt = time.Now()
}

// ID returns the Task's identifier.
func (t *Task) ID() ID {
	return t.id
}

// Title returns the Task's title.
func (t *Task) Title() Title {
	return t.title
}

// Status returns the Task's current status.
func (t *Task) Status() Status {
	return t.status
}

// Priority returns the Task's current priority.
func (t *Task) Priority() Priority {
	return t.priority
}

// CreatedAt returns the time the Task was created.
func (t *Task) CreatedAt() time.Time {
	return t.createdAt
}

// UpdatedAt returns the time the Task was last updated.
func (t *Task) UpdatedAt() time.Time {
	return t.updatedAt
}
