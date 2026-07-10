package client

import "context"

// Repository is the persistence boundary for the Client aggregate.
// It is defined in the domain layer (dependency inversion): the
// domain declares what it needs, and the infrastructure layer
// provides a concrete implementation, keeping the domain independent
// of any storage technology.
//
// FindByID returns ErrNotFound when no matching Client exists.
//
// Repository has no command methods, so it is a query-only port
// (Reader 相当・Writer を持たない) rather than being split into
// separate Reader/Writer interfaces (SPEC-010 R1: aggregates with no
// Save/Delete are left as-is instead of gaining an empty Writer). The
// composition root is free to wire it to a read-scoped connection
// pool.
type Repository interface {
	FindByID(ctx context.Context, id ClientID) (*Client, error)
}
