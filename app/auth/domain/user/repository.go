package user

import "context"

// Repository is the persistence boundary for the User aggregate. It
// is defined in the domain layer (dependency inversion): the domain
// declares what it needs, and the infrastructure layer provides a
// concrete implementation.
//
// FindByID and FindByUsername return ErrNotFound when no matching
// User exists.
//
// Repository has no command methods, so it is a query-only port
// (Reader 相当・Writer を持たない) rather than being split into
// separate Reader/Writer interfaces (SPEC-010 R1: aggregates with no
// Save/Delete are left as-is instead of gaining an empty Writer). The
// composition root is free to wire it to a read-scoped connection
// pool.
type Repository interface {
	FindByID(ctx context.Context, id UserID) (*User, error)
	FindByUsername(ctx context.Context, username Username) (*User, error)
	// ListAll returns every registered user ordered by id. An empty
	// store yields a nil slice, not an error.
	ListAll(ctx context.Context) ([]*User, error)
}

// Writer is the write-side persistence boundary for the User
// aggregate, introduced by ISSUE-039 to support the admin management
// API. It is kept separate from Repository (the read-only port) so
// the composition root can wire each to the appropriate connection
// pool.
//
// CreateUser upserts u (INSERT ... ON CONFLICT (id) DO UPDATE), so
// calling it multiple times with the same UserID converges
// idempotently on the latest state.
type Writer interface {
	CreateUser(ctx context.Context, u *User) error
}
