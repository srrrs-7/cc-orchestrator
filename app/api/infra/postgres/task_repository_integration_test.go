//go:build integration

// Package postgres_test holds the SPEC-005 integration suite for
// infra/postgres. It is gated behind the "integration" build tag so
// the default `make test` (no build tags, no DB required) stays
// green; it is meant to be run explicitly once impl-db wires it up as
// `make test-integration` (docs/plans/SPEC-005-plan.md §0 "Make
// ターゲット名"), e.g.:
//
//	go test -tags=integration ./infra/postgres/...
//
// against a live Postgres that already has `make migrate-up` applied
// (this suite does not run migrations itself -- see openTestDB).
//
// As of SPEC-005 phase B1 (TDD), infra/postgres does not exist yet:
// this file is written against its *planned* constructor,
// postgres.NewTaskRepository(db *sql.DB) (plan §2.1), and therefore
// intentionally fails to compile with -tags=integration until impl-db
// lands infra/postgres/task_repository.go. That is expected and does
// not affect the default (untagged) build/vet/test, which never
// parses this file.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/repotest"
)

// TestTaskRepository_Contract runs the same behavioral contract as
// infra/memory (infra/memory/task_repository_contract_test.go)
// against a real Postgres-backed task.Repository, proving R1
// ("infra/postgres の振る舞いは infra/memory と同じ").
func TestTaskRepository_Contract(t *testing.T) {
	repotest.RunTaskRepositoryContract(t, func(t *testing.T) task.Repository {
		db := openTestDB(t)
		truncateTasks(t, db)
		return postgres.NewTaskRepository(db)
	})
}

// TestTaskRepository_UniqueTitle_ViolatesConstraint is a Postgres-only
// test: unlike infra/memory (which silently allows two distinct Tasks
// to share a Title -- see docs/plans/SPEC-005-plan.md §6.1 R-a), the
// tasks.title column is expected to carry a UNIQUE constraint. This is
// deliberately NOT part of the shared repotest contract, since
// infra/memory does not (and per R-a, is not required to) exhibit
// this behavior.
//
// Expected mapping (impl-db): the Postgres unique_violation (SQLSTATE
// 23505) on tasks.title should be translated to the domain's existing
// task.ErrDuplicateTitle sentinel -- the same one route/response.go
// and service.TaskService already branch on via the application-level
// task.DuplicateChecker pre-check -- so infra/postgres's
// defense-in-depth failure mode matches the rest of the codebase's
// error taxonomy instead of leaking a raw database/sql or pgx error.
func TestTaskRepository_UniqueTitle_ViolatesConstraint(t *testing.T) {
	db := openTestDB(t)
	truncateTasks(t, db)
	repo := postgres.NewTaskRepository(db)
	ctx := context.Background()

	title, err := task.NewTitle("duplicate title")
	if err != nil {
		t.Fatalf("NewTitle() unexpected error: %v", err)
	}

	first := task.New(title, task.PriorityMedium)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save(first) unexpected error: %v", err)
	}

	second := task.New(title, task.PriorityMedium) // distinct ID, same Title
	err = repo.Save(ctx, second)
	if err == nil {
		t.Fatal("Save(second) with a duplicate title succeeded, want a UNIQUE constraint violation")
	}
	if !errors.Is(err, task.ErrDuplicateTitle) {
		t.Errorf("Save(second) error = %v, want wrapping %v", err, task.ErrDuplicateTitle)
	}
	// ISSUE-018: the unique_violation must also be recognized as the
	// *task.ConflictError category (HTTP 409), not just the bare
	// ErrDuplicateTitle sentinel, so route's errors.As type switch maps
	// it correctly.
	var ce *task.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("Save(second) error = %v, want errors.As(&task.ConflictError{}) = true", err)
	}
}

// TestTaskRepository_ErrorCategories_ISSUE018 covers ISSUE-018's
// category-type mapping for infra/postgres:
//   - not-found -> *task.NotFoundError (in addition to the shared
//     repotest contract's errors.Is(ErrNotFound) coverage);
//   - a corrupt row (bypassing domain validation at the DB layer) and
//     a forced driver failure both -> *task.DBError, and -- the
//     regression this Issue exists to prevent -- must NOT also
//     satisfy errors.As(&task.ValidationError{}). taskFromRow
//     deliberately severs an inner *task.ValidationError from the
//     Unwrap chain via %v (not %w) precisely so a corrupt DB row
//     surfaces as HTTP 500, not HTTP 400 (docs/plans/ISSUE-018-plan.md
//     "リスク / 未確定事項").
func TestTaskRepository_ErrorCategories_ISSUE018(t *testing.T) {
	t.Run("FindByID not found maps to *task.NotFoundError", func(t *testing.T) {
		db := openTestDB(t)
		truncateTasks(t, db)
		repo := postgres.NewTaskRepository(db)

		_, err := repo.FindByID(context.Background(), task.NewID())

		var nfe *task.NotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("FindByID() error = %v, want errors.As(&task.NotFoundError{}) = true", err)
		}
		if !errors.Is(err, task.ErrNotFound) {
			t.Errorf("FindByID() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("corrupt row (empty title bypassing domain validation) maps to *task.DBError, not *task.ValidationError", func(t *testing.T) {
		db := openTestDB(t)
		truncateTasks(t, db)
		ctx := context.Background()

		// Insert a row directly, bypassing task.NewTitle's
		// empty-title rejection: the tasks.title column only enforces
		// NOT NULL, not non-empty, so this models a row corrupted by
		// something other than this application (e.g. a manual data
		// fix, a different writer).
		id := task.NewID().String()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO tasks (id, title, status, priority, created_at, updated_at) VALUES ($1, '', 'todo', 'medium', now(), now())`,
			id,
		); err != nil {
			t.Fatalf("setup: insert corrupt row: %v", err)
		}

		repo := postgres.NewTaskRepository(db)
		parsedID, err := task.ParseID(id)
		if err != nil {
			t.Fatalf("setup: ParseID(%q) unexpected error: %v", id, err)
		}

		_, err = repo.FindByID(ctx, parsedID)
		if err == nil {
			t.Fatal("FindByID() on a corrupt row succeeded, want an error")
		}

		var dbErr *task.DBError
		if !errors.As(err, &dbErr) {
			t.Fatalf("FindByID() error = %v, want errors.As(&task.DBError{}) = true", err)
		}
		var ve *task.ValidationError
		if errors.As(err, &ve) {
			t.Errorf("FindByID() error = %v, want errors.As(&task.ValidationError{}) = false (corrupt row must map to 500, not 400)", err)
		}
	})

	t.Run("forced driver failure (closed connection) maps to *task.DBError", func(t *testing.T) {
		db := openTestDB(t)
		truncateTasks(t, db)
		repo := postgres.NewTaskRepository(db)
		if err := db.Close(); err != nil {
			t.Fatalf("setup: close db: %v", err)
		}

		_, err := repo.FindAll(context.Background())
		if err == nil {
			t.Fatal("FindAll() on a closed connection succeeded, want an error")
		}

		var dbErr *task.DBError
		if !errors.As(err, &dbErr) {
			t.Fatalf("FindAll() error = %v, want errors.As(&task.DBError{}) = true", err)
		}
		var ve *task.ValidationError
		if errors.As(err, &ve) {
			t.Errorf("FindAll() error = %v, want errors.As(&task.ValidationError{}) = false", err)
		}
	})
}

// openTestDB opens a *sql.DB against the Postgres instance described
// by the same discrete DB_* environment variables the application
// itself will use to configure persistence (docs/plans/SPEC-005-plan.md
// §0 "切替の env / DSN / 本番必須強制"). It skips the test (rather than
// failing) when DB_HOST is unset, so this file does not require a
// live database merely to be present in the tree; CI wires these
// variables to the postgres service container (plan §D2).
//
// It intentionally does not run goose migrations itself (see the
// package doc comment): the target database is expected to already
// have `make migrate-up` applied before this suite runs.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	host := os.Getenv("DB_HOST")
	if host == "" {
		t.Skip("DB_HOST not set; skipping infra/postgres integration test (see docs/plans/SPEC-005-plan.md §0)")
	}

	// "pgx" is registered as a database/sql driver name by
	// infra/postgres's own db.go (via a blank import of
	// github.com/jackc/pgx/v5/stdlib) once impl-db implements it; this
	// file deliberately never imports pgx directly, so it adds no
	// dependency of its own on go.mod/go.sum beyond this module's own
	// packages.
	db, err := sql.Open("pgx", testDSN())
	if err != nil {
		t.Fatalf("sql.Open(\"pgx\", ...) unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("db.PingContext() unexpected error: %v (is DB_* pointing at a reachable, migrated Postgres?)", err)
	}
	return db
}

// testDSN assembles a libpq-style connection string from the discrete
// DB_* environment variables (DB_HOST/DB_PORT/DB_NAME/DB_USER/
// DB_PASSWORD/DB_SSLMODE). DB_NAME defaults to "api" (this stack's own
// dedicated Postgres database, per the 2026-07-09 refactor, SPEC-005
// plan §RF.1.1: api and auth are separated by database, not by schema/
// search_path -- app/migrator creates and migrates it, see
// .claude/rules/db.md). The defaults below are fallbacks for local
// ad-hoc runs and are expected to mirror compose.yml's api service
// (impl-db); they carry no meaningful secret (matching values only
// exist in a local, disposable compose Postgres).
func testDSN() string {
	env := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	host := env("DB_HOST", "127.0.0.1")
	port := env("DB_PORT", "5432")
	name := env("DB_NAME", "api")
	user := env("DB_USER", "app")
	password := env("DB_PASSWORD", "app")
	sslmode := env("DB_SSLMODE", "disable")

	values := url.Values{}
	values.Set("sslmode", sslmode)

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     host + ":" + port,
		Path:     "/" + name,
		RawQuery: values.Encode(),
	}
	return u.String()
}

// truncateTasks empties the tasks table between subtests so each
// newRepo(t) call in the shared contract (see
// repotest.NewTaskRepository's doc comment) observes a store as empty
// as memory.NewTaskRepository()'s fresh map.
func truncateTasks(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "TRUNCATE TABLE tasks"); err != nil {
		t.Fatalf("truncate tasks: %v", err)
	}
}
