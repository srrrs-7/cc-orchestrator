package user

import "fmt"

// UserID is a value object uniquely identifying a User. It doubles as
// the OpenID Connect "sub" (subject) claim value.
type UserID struct {
	value string
}

// ParseUserID validates and wraps an existing string as a UserID. It
// rejects empty strings.
func ParseUserID(s string) (UserID, error) {
	if s == "" {
		return UserID{}, fmt.Errorf("user: parse user id: %w", ErrInvalidUserID)
	}
	return UserID{value: s}, nil
}

// String returns the underlying string representation of the UserID,
// which is also the OIDC "sub" claim value.
func (id UserID) String() string {
	return id.value
}
