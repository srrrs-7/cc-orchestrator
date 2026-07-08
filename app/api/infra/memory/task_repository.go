// Package memory provides an in-memory infrastructure-layer
// implementation of the task.Repository interface, useful for
// running the sample application without an external datastore.
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// TaskRepository is an in-memory, concurrency-safe implementation of
// task.Repository.
type TaskRepository struct {
	mu    sync.RWMutex
	tasks map[task.ID]*task.Task
}

// var _ task.Repository = (*TaskRepository)(nil) verifies at compile
// time that TaskRepository satisfies the domain's Repository
// interface.
var _ task.Repository = (*TaskRepository)(nil)

// NewTaskRepository builds an empty TaskRepository.
func NewTaskRepository() *TaskRepository {
	return &TaskRepository{
		tasks: make(map[task.ID]*task.Task),
	}
}

// Save inserts or updates t in the store. A clone is stored so that
// later mutations to the caller's *task.Task do not leak into the
// repository's internal state.
func (r *TaskRepository) Save(ctx context.Context, t *task.Task) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("memory: save task: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks[t.ID()] = clone(t)
	return nil
}

// FindByID returns the Task with the given id, or task.ErrNotFound
// if none exists.
func (r *TaskRepository) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find task by id: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tasks[id]
	if !ok {
		return nil, fmt.Errorf("memory: find task by id: %w", task.ErrNotFound)
	}
	return clone(t), nil
}

// FindByTitle returns the Task with the given title, or
// task.ErrNotFound if none exists.
func (r *TaskRepository) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find task by title: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.tasks {
		if t.Title() == title {
			return clone(t), nil
		}
	}
	return nil, fmt.Errorf("memory: find task by title: %w", task.ErrNotFound)
}

// FindAll returns every Task currently stored.
func (r *TaskRepository) FindAll(ctx context.Context) ([]*task.Task, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find all tasks: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*task.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		result = append(result, clone(t))
	}
	return result, nil
}

// clone returns a new *task.Task built from t's state via
// task.Reconstruct, preventing the repository's stored data and the
// caller's data from aliasing (sharing) the same mutable object.
func clone(t *task.Task) *task.Task {
	return task.Reconstruct(t.ID(), t.Title(), t.Status(), t.Priority(), t.CreatedAt(), t.UpdatedAt())
}
