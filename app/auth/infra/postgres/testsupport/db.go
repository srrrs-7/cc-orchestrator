//go:build integration

// Package testsupport provides shared test-DB helpers for
// infra/postgres integration tests (build tag: integration).
//
// It consolidates the openTestDB / truncateTable / testConfig helpers
// that were previously defined in infra/postgres/testdb_integration_test.go,
// and makes them available as an importable package so that route
// integration tests (SPEC-011 Phase 2) can reuse the same connection,
// truncation, and demo-data seeding logic without rebuilding it.
//
// This package is tagged //go:build integration so it is never compiled
// in the default `make test` (offline, no DB). Any file that imports
// this package must also be tagged integration.
//
// Importing this package also imports infra/postgres, which blank-imports
// github.com/jackc/pgx/v5/stdlib, registering "pgx" as the
// database/sql driver used by OpenTestDB.
package testsupport

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
)

// OpenTestDB opens a *sql.DB against the Postgres instance described
// by the discrete DB_* environment variables (DB_HOST / DB_PORT /
// DB_NAME / DB_USER / DB_PASSWORD / DB_SSLMODE). It skips the test
// (rather than failing) when DB_HOST is unset, so integration-tagged
// files compile and are ignored on machines without a live Postgres.
//
// DB_NAME defaults to "auth" (this stack's own dedicated Postgres
// database, per SPEC-005 plan §RF.1.1: api and auth are separated by
// database, not by schema / search_path; app/migrator creates and
// migrates it, see .claude/rules/db.md).
//
// The returned *sql.DB is registered for cleanup via t.Cleanup; callers
// must not call db.Close() themselves.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	host := os.Getenv("DB_HOST")
	if host == "" {
		t.Skip("DB_HOST not set; skipping integration test (see docs/plans/SPEC-005-plan.md §0)")
	}

	db, err := sql.Open("pgx", TestConfig().DSN())
	if err != nil {
		t.Fatalf("sql.Open(\"pgx\", ...) unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("db.PingContext() unexpected error: %v (is DB_* pointing at a reachable, migrated Postgres?)", err)
	}
	return db
}

// TestConfig builds a postgres.Config from the same discrete DB_*
// environment variables the application itself uses to configure
// persistence. DB_NAME defaults to "auth".
func TestConfig() postgres.Config {
	env := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	return postgres.Config{
		Host:     env("DB_HOST", "127.0.0.1"),
		Port:     env("DB_PORT", "5432"),
		Name:     env("DB_NAME", "auth"),
		User:     env("DB_USER", "app"),
		Password: env("DB_PASSWORD", "app"),
		SSLMode:  env("DB_SSLMODE", "disable"),
	}
}

// TruncateTable empties a table between subtests so each newRepo(t)
// call observes a store as empty as a fresh in-memory map. table is
// always one of this package's own hard-coded literals (never user
// input), so building the statement via string concatenation carries
// no injection risk.
func TruncateTable(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "TRUNCATE TABLE "+table); err != nil {
		t.Fatalf("truncate %s: %v", table, err)
	}
}

// SeedClient idempotently inserts (or overwrites, keyed by ID) c into
// the clients table via postgres.SeedClient's upsert. It fails the
// test on error, so callers can treat seeding failures as fatal setup
// errors without boilerplate error handling.
func SeedClient(t *testing.T, db *sql.DB, c *client.Client) {
	t.Helper()
	if err := postgres.SeedClient(t.Context(), db, c); err != nil {
		t.Fatalf("SeedClient(%v) unexpected error: %v", c.ID(), err)
	}
}

// SeedUser idempotently inserts (or overwrites, keyed by ID) u into
// the users table via postgres.SeedUser's upsert. It fails the test
// on error, so callers can treat seeding failures as fatal setup
// errors without boilerplate error handling.
func SeedUser(t *testing.T, db *sql.DB, u *user.User) {
	t.Helper()
	if err := postgres.SeedUser(t.Context(), db, u); err != nil {
		t.Fatalf("SeedUser(%v) unexpected error: %v", u.ID(), err)
	}
}
