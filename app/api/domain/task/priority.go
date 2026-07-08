package task

import "fmt"

// Priority is a value object representing a Task's priority.
type Priority struct {
	value string
}

// The three valid Priority values a Task can hold.
var (
	PriorityLow    = Priority{value: "low"}
	PriorityMedium = Priority{value: "medium"}
	PriorityHigh   = Priority{value: "high"}
)

// String returns the underlying string representation of the Priority.
func (p Priority) String() string {
	return p.value
}

// ParsePriority validates and converts s into a Priority. It returns
// ErrInvalidPriority if s does not match any known priority value.
// The empty string is invalid (defaulting to medium on task creation
// is an application-layer concern, not a domain one).
func ParsePriority(s string) (Priority, error) {
	switch s {
	case PriorityLow.value:
		return PriorityLow, nil
	case PriorityMedium.value:
		return PriorityMedium, nil
	case PriorityHigh.value:
		return PriorityHigh, nil
	default:
		return Priority{}, fmt.Errorf("task: parse priority %q: %w", s, ErrInvalidPriority)
	}
}
