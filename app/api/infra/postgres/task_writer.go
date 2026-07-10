package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres/sqlcgen"
)

// TaskWriter is a Postgres-backed implementation of task.Writer
// (SPEC-010 R1/R2). It holds only the *sqlcgen.Queries handle bound
// to the *sql.DB pool the composition root (cmd/api/main.go) intends
// for write traffic (postgres.OpenPair's writer pool). See
// TaskReader's doc comment for why splitting the reader and writer
// into separate structs, each bound to exactly one pool, is what
// makes the SPEC-010 read/write pool routing work.
type TaskWriter struct {
	q *sqlcgen.Queries
}

// var _ task.Writer = (*TaskWriter)(nil) verifies at compile time that
// TaskWriter satisfies the domain's Writer interface.
var _ task.Writer = (*TaskWriter)(nil)

// NewTaskWriter builds a TaskWriter backed by db. db is not owned by
// the returned TaskWriter: the caller (cmd/api/main.go, typically via
// postgres.OpenPair) is responsible for opening and closing it.
func NewTaskWriter(db *sql.DB) *TaskWriter {
	return &TaskWriter{q: sqlcgen.New(db)}
}

// Save inserts or updates t in the tasks table (UpsertTask: INSERT ...
// ON CONFLICT (id) DO UPDATE), matching infra/memory's upsert-by-id
// semantics. A UNIQUE(title) violation -- saving a Task whose title
// already belongs to a different id -- is translated to a
// *task.ConflictError wrapping task.ErrDuplicateTitle (via
// task.NewDuplicateTitleError) rather than leaking a raw database/sql
// or pgx error, so callers can branch with errors.Is/errors.As the
// same way they do against task.DuplicateChecker's pre-check. Any
// other database error is wrapped as a *task.DBError.
func (r *TaskWriter) Save(ctx context.Context, t *task.Task) error {
	err := r.q.UpsertTask(ctx, sqlcgen.UpsertTaskParams{
		ID:        t.ID().String(),
		Title:     t.Title().String(),
		Status:    t.Status().String(),
		Priority:  t.Priority().String(),
		CreatedAt: t.CreatedAt(),
		UpdatedAt: t.UpdatedAt(),
	})
	if err != nil {
		if isUniqueViolation(err, tasksTitleUniqueConstraint) {
			return fmt.Errorf("postgres: save task: %w", task.NewDuplicateTitleError())
		}
		return fmt.Errorf("postgres: save task: %w", task.NewDBError(err))
	}
	return nil
}
