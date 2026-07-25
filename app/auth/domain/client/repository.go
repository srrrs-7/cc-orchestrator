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
	// ListAll returns every registered client ordered by id. An empty
	// store yields a nil slice, not an error.
	ListAll(ctx context.Context) ([]*Client, error)
}

// Writer is the write-side persistence boundary for the Client
// aggregate, introduced by ISSUE-039 to support the admin management
// API. It is kept separate from Repository (the read-only port) so
// the composition root can wire each to the appropriate connection
// pool (writer pool for writes, reader pool for reads).
//
// Save upserts c (INSERT ... ON CONFLICT DO UPDATE), so calling it
// multiple times with the same ClientID converges idempotently on the
// latest state rather than erroring on the second call.
type Writer interface {
	Save(ctx context.Context, c *Client) error
	// DeleteClient removes c and any dependent consent, refresh-token, and
	// authorization-code rows. Returns ErrNotFound when id is absent.
	DeleteClient(ctx context.Context, id ClientID) error
}
