// Package memory provides an in-memory infrastructure-layer
// implementation of the task.Repository interface, useful for
// running the sample application without an external datastore.
package memory

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// TaskRepository is an in-memory, concurrency-safe implementation of
// task.Repository.
type TaskRepository struct {
	mu    sync.RWMutex
	tasks map[task.ID]*task.Task
}

// TaskRepository stays a single struct implementing every method of
// both task.Reader and task.Writer (SPEC-010 R5: in-memory storage is
// not physically split into separate read/write pools -- there is
// only one map to read and write). These three var _ declarations
// verify at compile time that it still satisfies task.Repository as a
// whole, and each of task.Reader/task.Writer individually, with no
// behavior change.
var (
	_ task.Repository = (*TaskRepository)(nil)
	_ task.Reader     = (*TaskRepository)(nil)
	_ task.Writer     = (*TaskRepository)(nil)
)

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
		return fmt.Errorf("memory: save task: %w", task.NewDBError(ctx.Err()))
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
		return nil, fmt.Errorf("memory: find task by id: %w", task.NewDBError(ctx.Err()))
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tasks[id]
	if !ok {
		return nil, fmt.Errorf("memory: find task by id: %w", task.NewNotFoundError())
	}
	return clone(t), nil
}

// FindByTitle returns the Task with the given title, or
// task.ErrNotFound if none exists.
func (r *TaskRepository) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find task by title: %w", task.NewDBError(ctx.Err()))
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.tasks {
		if t.Title() == title {
			return clone(t), nil
		}
	}
	return nil, fmt.Errorf("memory: find task by title: %w", task.NewNotFoundError())
}

// ListPage returns a stable-ordered (created_at, id ascending) page
// of Tasks per page's limit/offset, along with total: the number of
// Tasks currently stored, independent of the page window (SPEC-008).
// An offset at or beyond total yields an empty items slice, not an
// error. The map that backs TaskRepository has no inherent order, so
// every call sorts a full snapshot before slicing the requested
// window -- this mirrors infra/postgres's `ORDER BY created_at, id`
// and keeps page boundaries free of duplicates/gaps.
func (r *TaskRepository) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	select {
	case <-ctx.Done():
		return nil, 0, fmt.Errorf("memory: list tasks: %w", task.NewDBError(ctx.Err()))
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*task.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		all = append(all, t)
	}
	slices.SortFunc(all, func(a, b *task.Task) int {
		if c := a.CreatedAt().Compare(b.CreatedAt()); c != 0 {
			return c
		}
		return strings.Compare(a.ID().String(), b.ID().String())
	})

	total := len(all)
	start := min(page.Offset(), total)
	end := min(start+page.Limit(), total)

	result := make([]*task.Task, 0, end-start)
	for _, t := range all[start:end] {
		result = append(result, clone(t))
	}
	return result, total, nil
}

// clone returns a new *task.Task built from t's state via
// task.Reconstruct, preventing the repository's stored data and the
// caller's data from aliasing (sharing) the same mutable object.
func clone(t *task.Task) *task.Task {
	return task.Reconstruct(t.ID(), t.Title(), t.Status(), t.Priority(), t.CreatedAt(), t.UpdatedAt())
}
