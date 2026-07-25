// Package user contains the User aggregate (an OIDC resource
// owner / subject) and its supporting domain types (value objects,
// repository interface and domain errors). This package has no
// dependency on any other layer, and it does not depend on any other
// bounded context in this system.
package user

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// User is the aggregate root representing a resource owner /
// OpenID Connect subject. Its UserID doubles as the "sub" claim
// issued in ID Tokens and returned from the UserInfo endpoint.
type User struct {
	id           UserID
	username     Username
	passwordHash string
	profile      Profile
}

// New is the factory for registering a brand new User. plaintextPassword
// is hashed with bcrypt before storage; callers must never persist the
// returned hash via PasswordHash() outside infrastructure seed paths.
func New(id UserID, username Username, plaintextPassword string, profile Profile) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	return &User{
		id:           id,
		username:     username,
		passwordHash: string(hash),
		profile:      profile,
	}, nil
}

// Reconstruct rebuilds a User from already-validated persisted state.
// passwordHash must be the bcrypt hash loaded from storage, not plaintext.
// It is intended to be used exclusively by infrastructure-layer
// repository implementations when loading a User from storage.
func Reconstruct(id UserID, username Username, passwordHash string, profile Profile) *User {
	return &User{
		id:           id,
		username:     username,
		passwordHash: passwordHash,
		profile:      profile,
	}
}

// VerifyPassword reports whether candidate matches the User's stored
// bcrypt hash using constant-time comparison via bcrypt.
func (u *User) VerifyPassword(candidate string) bool {
	if candidate == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.passwordHash), []byte(candidate)) == nil
}

// ID returns the User's identifier (the OIDC "sub" value).
func (u *User) ID() UserID {
	return u.id
}

// Username returns the User's login name.
func (u *User) Username() Username {
	return u.username
}

// Profile returns the User's profile (name/email).
func (u *User) Profile() Profile {
	return u.profile
}

// PasswordHash returns the User's stored bcrypt hash. It is exposed
// primarily so infrastructure-layer repositories can persist the
// aggregate; application code should prefer VerifyPassword.
func (u *User) PasswordHash() string {
	return u.passwordHash
}
