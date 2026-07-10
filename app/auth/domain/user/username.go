package user

import (
	"fmt"
	"strings"
)

// Username is a value object representing a User's login name (used
// e.g. to resolve the "login_hint" authorize parameter).
type Username struct {
	value string
}

// NewUsername trims surrounding whitespace and validates s before
// constructing a Username. It returns ErrInvalidUsername if the
// trimmed value is empty.
func NewUsername(s string) (Username, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return Username{}, fmt.Errorf("user: new username: %w", ErrInvalidUsername)
	}
	return Username{value: trimmed}, nil
}

// String returns the underlying string representation of the
// Username.
func (u Username) String() string {
	return u.value
}
