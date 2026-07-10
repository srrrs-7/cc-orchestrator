// repository_test.go is a compile-time-only proof of SPEC-010 R1 for
// the task aggregate: task.Repository (domain/task/repository.go) must
// split additively into task.Reader (FindByID/FindByTitle/ListPage)
// and task.Writer (Save), with task.Repository remaining exactly their
// composition (interface{ Reader; Writer }). It asserts nothing at
// runtime -- if this package compiles, the three interfaces have
// exactly the shape this file's fakes assume.
package task_test

import (
	"context"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// repoReaderOnly implements only the three methods task.Reader
// declares (FindByID, FindByTitle, ListPage) and has no Save method.
// Assigning it to a task.Reader-typed variable is a compile-time proof
// that Reader requires nothing beyond those three methods.
type repoReaderOnly struct{}

func (repoReaderOnly) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	return nil, task.ErrNotFound
}

func (repoReaderOnly) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	return nil, task.ErrNotFound
}

func (repoReaderOnly) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	return nil, 0, nil
}

// repoWriterOnly implements only task.Writer's single method (Save)
// and has no Find*/ListPage methods at all. Assigning it to a
// task.Writer-typed variable is a compile-time proof that Writer
// requires nothing beyond Save.
type repoWriterOnly struct{}

func (repoWriterOnly) Save(ctx context.Context, t *task.Task) error { return nil }

// repoReaderWriter embeds both narrow fakes above, promoting their
// methods, so it satisfies the full set FindByID/FindByTitle/
// ListPage/Save.
type repoReaderWriter struct {
	repoReaderOnly
	repoWriterOnly
}

// TestRepository_ReaderWriterSplit is SPEC-010 R1's compile-time
// contract for the task aggregate: Reader and Writer must be exactly
// as narrow as documented, and their composition must still satisfy
// the pre-existing Repository interface every current consumer
// (service.TaskService's compatible constructor, repotest, infra/
// memory, infra/postgres) depends on.
func TestRepository_ReaderWriterSplit(t *testing.T) {
	var _ task.Reader = repoReaderOnly{}
	var _ task.Writer = repoWriterOnly{}
	var _ task.Repository = repoReaderWriter{}
}
