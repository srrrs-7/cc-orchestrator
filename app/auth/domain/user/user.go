// Package user contains the User aggregate (an OIDC resource
// owner / subject) and its supporting domain types (value objects,
// repository interface and domain errors). This package has no
// dependency on any other layer, and it does not depend on any other
// bounded context in this system.
package user

// User is the aggregate root representing a resource owner /
// OpenID Connect subject. Its UserID doubles as the "sub" claim
// issued in ID Tokens and returned from the UserInfo endpoint.
type User struct {
	id       UserID
	username Username
	password string
	profile  Profile
}

// New is the factory for registering a brand new User. password is
// stored as-is for this demo authorization server (see
// VerifyPassword); a production system must hash it.
func New(id UserID, username Username, password string, profile Profile) *User {
	return &User{id: id, username: username, password: password, profile: profile}
}

// Reconstruct rebuilds a User from already-validated persisted state.
// It is intended to be used exclusively by infrastructure-layer
// repository implementations when loading a User from storage.
func Reconstruct(id UserID, username Username, password string, profile Profile) *User {
	return New(id, username, password, profile)
}

// VerifyPassword reports whether candidate matches the User's stored
// password.
//
// This is a deliberately simplified, plaintext comparison: this
// sample authorization server does not implement a real login/consent
// UI (see route.authorizeHandler for where one would be added), so
// there is no login form that ever calls this in the current wiring.
// It is kept here so the aggregate's shape matches a real IdP and can
// be wired to an actual login handler.
func (u *User) VerifyPassword(candidate string) bool {
	return candidate != "" && candidate == u.password
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

// Password returns the User's stored credential. It is exposed
// primarily so infrastructure-layer repositories can reconstruct a
// clone of the aggregate for storage isolation; application code
// should prefer VerifyPassword.
func (u *User) Password() string {
	return u.password
}
