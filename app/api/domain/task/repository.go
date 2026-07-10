package task

import "context"

// Repository is the persistence boundary for the Task aggregate.
// It is defined in the domain layer (dependency inversion): the
// domain declares what it needs, and the infrastructure layer
// provides a concrete implementation, keeping the domain independent
// of any storage technology.
//
// FindByID and FindByTitle return ErrNotFound when no matching Task
// exists.
type Repository interface {
	Save(ctx context.Context, t *Task) error
	FindByID(ctx context.Context, id ID) (*Task, error)
	FindByTitle(ctx context.Context, title Title) (*Task, error)

	// ListPage returns a stable-ordered (created_at, id ascending)
	// page of Tasks per page's limit/offset, along with total: the
	// number of Tasks in the store independent of the page window
	// (SPEC-008 R2). An offset at or beyond total yields an empty
	// items slice, not an error.
	ListPage(ctx context.Context, page Page) (items []*Task, total int, err error)
}
