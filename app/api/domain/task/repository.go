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
	FindAll(ctx context.Context) ([]*Task, error)
}
