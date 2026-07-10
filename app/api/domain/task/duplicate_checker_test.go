package task_test

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// fakeRepository is a minimal task.Repository fake used to exercise
// the DuplicateChecker domain service without depending on the infra
// layer. Only FindByTitle is exercised by DuplicateChecker; the other
// methods are unused stubs.
type fakeRepository struct {
	findByTitleFunc func(ctx context.Context, title task.Title) (*task.Task, error)
}

func (f *fakeRepository) Save(ctx context.Context, t *task.Task) error { return nil }

func (f *fakeRepository) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	return nil, task.ErrNotFound
}

func (f *fakeRepository) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	return f.findByTitleFunc(ctx, title)
}

func (f *fakeRepository) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	return nil, 0, nil
}

var errRepoBoom = errors.New("boom")

func TestDuplicateChecker_IsDuplicated(t *testing.T) {
	title, err := task.NewTitle("buy milk")
	if err != nil {
		t.Fatalf("NewTitle() unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		repo    *fakeRepository
		wantDup bool
		wantErr error
	}{
		{
			name: "no matching task is not a duplicate",
			repo: &fakeRepository{
				findByTitleFunc: func(ctx context.Context, title task.Title) (*task.Task, error) {
					return nil, task.ErrNotFound
				},
			},
			wantDup: false,
		},
		{
			name: "matching task is a duplicate",
			repo: &fakeRepository{
				findByTitleFunc: func(ctx context.Context, title task.Title) (*task.Task, error) {
					return task.New(title, task.PriorityMedium), nil
				},
			},
			wantDup: true,
		},
		{
			name: "repository error propagates",
			repo: &fakeRepository{
				findByTitleFunc: func(ctx context.Context, title task.Title) (*task.Task, error) {
					return nil, errRepoBoom
				},
			},
			wantErr: errRepoBoom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := task.NewDuplicateChecker(tt.repo)

			dup, err := checker.IsDuplicated(context.Background(), title)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("IsDuplicated() error = %v, want wrapping %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("IsDuplicated() unexpected error: %v", err)
			}
			if dup != tt.wantDup {
				t.Errorf("IsDuplicated() = %v, want %v", dup, tt.wantDup)
			}
		})
	}
}
