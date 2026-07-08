package user

import (
	"fmt"
	"strings"
)

// Profile is a value object holding the OIDC "profile"/"email" scope
// claims for a User: display name and email address.
type Profile struct {
	name  string
	email string
}

// NewProfile validates name and email and constructs a Profile. It
// returns ErrInvalidEmail if email does not contain exactly one "@"
// with non-empty local and domain parts (a deliberately simple check
// suitable for a demo authorization server).
func NewProfile(name, email string) (Profile, error) {
	if !looksLikeEmail(email) {
		return Profile{}, fmt.Errorf("user: new profile: %w", ErrInvalidEmail)
	}
	return Profile{name: strings.TrimSpace(name), email: email}, nil
}

func looksLikeEmail(email string) bool {
	at := strings.Index(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	return !strings.Contains(email[at+1:], "@")
}

// Name returns the profile's display name.
func (p Profile) Name() string {
	return p.name
}

// Email returns the profile's email address.
func (p Profile) Email() string {
	return p.email
}
