package task

import "fmt"

// Status is a value object representing the lifecycle state of a Task.
type Status struct {
	value string
}

// The three valid Status values a Task can hold.
var (
	StatusTodo  = Status{value: "todo"}
	StatusDoing = Status{value: "doing"}
	StatusDone  = Status{value: "done"}
)

// CanTransitionTo reports whether transitioning from the receiver
// Status to next is allowed. Only todo -> doing and doing -> done
// are valid transitions.
func (s Status) CanTransitionTo(next Status) bool {
	switch {
	case s == StatusTodo && next == StatusDoing:
		return true
	case s == StatusDoing && next == StatusDone:
		return true
	default:
		return false
	}
}

// String returns the underlying string representation of the Status.
func (s Status) String() string {
	return s.value
}

// ParseStatus validates and converts s into a Status. It returns
// ErrInvalidStatus if s does not match any known status value.
func ParseStatus(s string) (Status, error) {
	switch s {
	case StatusTodo.value:
		return StatusTodo, nil
	case StatusDoing.value:
		return StatusDoing, nil
	case StatusDone.value:
		return StatusDone, nil
	default:
		return Status{}, fmt.Errorf("task: parse status %q: %w", s, ErrInvalidStatus)
	}
}
