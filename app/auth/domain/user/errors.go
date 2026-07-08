package user

import "errors"

// Sentinel errors returned by the user domain package. Callers should
// use errors.Is to branch on these, since they may be wrapped with
// additional context via fmt.Errorf("...: %w", err).
var (
	// ErrNotFound is returned by Repository lookups when no matching
	// User exists.
	ErrNotFound = errors.New("user: not found")

	// ErrInvalidUserID is returned when a UserID cannot be parsed from
	// a string (e.g. it is empty).
	ErrInvalidUserID = errors.New("user: invalid user id")

	// ErrInvalidUsername is returned when a Username is empty.
	ErrInvalidUsername = errors.New("user: invalid username")

	// ErrInvalidEmail is returned when a Profile's email does not look
	// like a valid email address.
	ErrInvalidEmail = errors.New("user: invalid email")
)
