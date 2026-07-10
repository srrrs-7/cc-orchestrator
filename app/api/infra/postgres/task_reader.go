package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres/sqlcgen"
)

// TaskReader is a Postgres-backed implementation of task.Reader
// (SPEC-010 R1/R2). It holds only the *sqlcgen.Queries handle bound
// to the *sql.DB pool the composition root (cmd/api/main.go) intends
// for read traffic -- ordinarily the same pool TaskWriter uses (when
// DB_READER_* is unset, postgres.OpenPair shares a single *sql.DB),
// but potentially a distinct reader pool (a future read replica
// endpoint) once DB_READER_HOST is configured. Splitting TaskReader
// out from TaskWriter as its own struct is what makes that routing
// possible: each struct is bound to exactly one pool at construction
// time, so which pool a query lands on falls directly out of which
// struct the caller (service.TaskService, task.DuplicateChecker) was
// given, not any runtime branching inside this package.
type TaskReader struct {
	q *sqlcgen.Queries
}

// var _ task.Reader = (*TaskReader)(nil) verifies at compile time that
// TaskReader satisfies the domain's Reader interface.
var _ task.Reader = (*TaskReader)(nil)

// NewTaskReader builds a TaskReader backed by db. db is not owned by
// the returned TaskReader: the caller (cmd/api/main.go, typically via
// postgres.OpenPair) is responsible for opening and closing it.
func NewTaskReader(db *sql.DB) *TaskReader {
	return &TaskReader{q: sqlcgen.New(db)}
}

// FindByID returns the Task with the given id, or a *task.NotFoundError
// (unwrapping to task.ErrNotFound) if none exists. Any other database
// error is wrapped as a *task.DBError.
func (r *TaskReader) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	return findTask("find task by id", func() (sqlcgen.Task, error) {
		return r.q.GetTaskByID(ctx, id.String())
	})
}

// FindByTitle returns the Task with the given title, or a
// *task.NotFoundError (unwrapping to task.ErrNotFound) if none exists.
// Any other database error is wrapped as a *task.DBError.
func (r *TaskReader) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	return findTask("find task by title", func() (sqlcgen.Task, error) {
		return r.q.GetTaskByTitle(ctx, title.String())
	})
}

// findTask runs a single-row task lookup (query) and decodes the result
// into a domain *task.Task, applying the error taxonomy every
// TaskReader point lookup shares: sql.ErrNoRows becomes a
// *task.NotFoundError (unwrapping to task.ErrNotFound), any other query
// error a *task.DBError, and a row that fails domain re-validation the
// error taskFromRow already returns -- each wrapped with the same
// "postgres: <op>: " context prefix. FindByID and FindByTitle differ
// only in the query they run and their op label, so they share this
// helper rather than repeat the ladder.
func findTask(op string, query func() (sqlcgen.Task, error)) (*task.Task, error) {
	row, err := query()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: %s: %w", op, task.NewNotFoundError())
		}
		return nil, fmt.Errorf("postgres: %s: %w", op, task.NewDBError(err))
	}
	t, err := taskFromRow(row)
	if err != nil {
		return nil, fmt.Errorf("postgres: %s: %w", op, err)
	}
	return t, nil
}

// ListPage returns the Tasks in page -- ordered by created_at, id
// ascending, at most page.Limit() rows starting at page.Offset() (see
// db/queries/tasks.sql: ListTasksPage) -- alongside the total number
// of Tasks in the table regardless of limit/offset (CountTasks). An
// offset at or beyond the total simply yields an empty items slice,
// not an error (SPEC-008 R2/R5).
//
// CountTasks and ListTasksPage are two separate statements rather than
// a single windowed query, so under concurrent writes the returned
// total and items can drift by a small amount relative to each other;
// this is accepted at this sample's scale (SPEC-008 plan risk R-6).
// Any database error from either query is wrapped as a *task.DBError.
func (r *TaskReader) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	total, err := r.q.CountTasks(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list tasks page: count: %w", task.NewDBError(err))
	}

	rows, err := r.q.ListTasksPage(ctx, sqlcgen.ListTasksPageParams{
		Limit:  int64(page.Limit()),
		Offset: int64(page.Offset()),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list tasks page: %w", task.NewDBError(err))
	}

	result := make([]*task.Task, 0, len(rows))
	for _, row := range rows {
		t, err := taskFromRow(row)
		if err != nil {
			return nil, 0, fmt.Errorf("postgres: list tasks page: %w", err)
		}
		result = append(result, t)
	}
	return result, int(total), nil
}
