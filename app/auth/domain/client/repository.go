package client

import "context"

// Repository is the persistence boundary for the Client aggregate.
// It is defined in the domain layer (dependency inversion): the
// domain declares what it needs, and the infrastructure layer
// provides a concrete implementation, keeping the domain independent
// of any storage technology.
//
// FindByID returns ErrNotFound when no matching Client exists.
type Repository interface {
	FindByID(ctx context.Context, id ClientID) (*Client, error)
}
