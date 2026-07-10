package user

import "context"

// Repository is the persistence boundary for the User aggregate. It
// is defined in the domain layer (dependency inversion): the domain
// declares what it needs, and the infrastructure layer provides a
// concrete implementation.
//
// FindByID and FindByUsername return ErrNotFound when no matching
// User exists.
type Repository interface {
	FindByID(ctx context.Context, id UserID) (*User, error)
	FindByUsername(ctx context.Context, username Username) (*User, error)
}
