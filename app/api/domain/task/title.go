package task

import (
	"fmt"
	"strings"
)

// maxTitleRunes is the maximum number of runes allowed in a Title.
const maxTitleRunes = 100

// Title is a value object representing a Task's title.
// It is immutable and guarantees its own invariants
// (non-empty, bounded length) at construction time.
type Title struct {
	value string
}

// NewTitle trims surrounding whitespace and validates s before
// constructing a Title. It returns ErrEmptyTitle if the trimmed
// value is empty, and ErrTitleTooLong if it exceeds maxTitleRunes.
func NewTitle(s string) (Title, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return Title{}, fmt.Errorf("task: new title: %w", ErrEmptyTitle)
	}
	if len([]rune(trimmed)) > maxTitleRunes {
		return Title{}, fmt.Errorf("task: new title: %w", ErrTitleTooLong)
	}
	return Title{value: trimmed}, nil
}

// String returns the underlying string representation of the Title.
func (t Title) String() string {
	return t.value
}
