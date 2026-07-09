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
// DB_PASSWORD/DB_SSLMODE/DB_SCHEMA). DB_SCHEMA is applied via
// search_path so the suite runs against the "api" schema (SPEC-005
// R3), matching the non-qualified migrations/queries (plan §0
// "スキーマ分離機構"). The defaults below are fallbacks for local ad-hoc
// runs and are expected to mirror docker/postgres/initdb (impl-db);
// they carry no meaningful secret (matching values only exist in a
// local, disposable compose Postgres).
func testDSN() string {
	env := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	host := env("DB_HOST", "127.0.0.1")
	port := env("DB_PORT", "5432")
	name := env("DB_NAME", "app")
	user := env("DB_USER", "app")
	password := env("DB_PASSWORD", "app")
	sslmode := env("DB_SSLMODE", "disable")
	schema := env("DB_SCHEMA", "api")

	values := url.Values{}
	values.Set("sslmode", sslmode)
	values.Set("search_path", schema)

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
