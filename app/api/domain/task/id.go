package task

import (
	"crypto/rand"
	"fmt"
)

// ID is a value object that uniquely identifies a Task.
// It is immutable; once created its value never changes.
type ID struct {
	value string
}

// NewID generates a new ID using a random UUIDv4-formatted string.
func NewID() ID {
	var buf [16]byte
	// crypto/rand.Read never returns an error on supported platforms;
	// if it ever fails, the returned buf is left zero-valued which is
	// acceptable for a sample implementation.
	_, _ = rand.Read(buf[:])

	// Set version (4) and variant (RFC 4122) bits.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	value := fmt.Sprintf("%x-%x-%x-%x-%x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])

	return ID{value: value}
}

// ParseID validates and wraps an existing string as an ID.
// It rejects empty strings.
func ParseID(s string) (ID, error) {
	if s == "" {
		return ID{}, &ValidationError{Msg: "invalid task id", Err: ErrInvalidID}
	}
	return ID{value: s}, nil
}

// String returns the underlying string representation of the ID.
func (i ID) String() string {
	return i.value
}
