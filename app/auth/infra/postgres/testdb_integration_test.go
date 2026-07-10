//go:build integration

// Package postgres_test holds the SPEC-005 integration suite for
// app/auth's infra/postgres. It is gated behind the "integration"
// build tag so the default `make test` (no build tags, no DB
// required) stays green; it is meant to be run explicitly once
// impl-db wires it up as `make test-integration`
// (docs/plans/SPEC-005-plan.md §0 "Make ターゲット名"), e.g.:
//
//	go test -tags=integration ./infra/postgres/...
//
// against a live Postgres that already has `make migrate-up` applied
// (this suite does not run migrations itself -- see openTestDB).
//
// As of SPEC-005 phase B1 (TDD), infra/postgres does not exist yet:
// the *_integration_test.go files in this package are written against
// its *planned* constructors -- postgres.NewClientRepository(db),
// postgres.NewUserRepository(db), postgres.NewAuthCodeRepository(db)
// (plan §2.2) -- and therefore intentionally fail to compile with
// -tags=integration until impl-db lands infra/postgres. That is
// expected and does not affect the default (untagged) build/vet/test,
// which never parses these files.
package postgres_test

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"testing"
)

// openTestDB opens a *sql.DB against the Postgres instance described
// by the same discrete DB_* environment variables the application
// itself will use to configure persistence (docs/plans/SPEC-005-plan.md
// §0 "切替の env / DSN / 本番必須強制"). It skips the test (rather than
// failing) when DB_HOST is unset, so these files do not require a
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
// DB_PASSWORD/DB_SSLMODE). DB_NAME defaults to "auth" (this stack's
// own dedicated Postgres database, per the 2026-07-09 refactor,
// SPEC-005 plan §RF.1.1: api and auth are separated by database, not
// by schema/search_path -- app/migrator creates and migrates it, see
// .claude/rules/db.md). The defaults below are fallbacks for local
// ad-hoc runs and are expected to mirror compose.yml's auth service
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
	name := env("DB_NAME", "auth")
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

// truncateTable empties table between subtests so each newRepo(t)
// call in the shared contracts (see e.g. repotest.NewClientRepository's
// doc comment) observes a store as empty as
// memory.NewClientRepository()'s/NewUserRepository()'s/
// NewAuthCodeRepository()'s fresh map. table is always one of this
// package's own hard-coded literals (never user input), so building
// the statement via string concatenation (TRUNCATE does not support
// parameter placeholders for identifiers) carries no injection risk.
func truncateTable(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "TRUNCATE TABLE "+table); err != nil {
		t.Fatalf("truncate %s: %v", table, err)
	}
}
