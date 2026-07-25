// Package testsupport provides shared test-DB helpers for
// infra/postgres tests.
//
// It consolidates the openTestDB / truncateTable / testConfig helpers
// that were previously defined in infra/postgres/testdb_integration_test.go,
// and makes them available as an importable package so that route
// tests can reuse the same connection, truncation, and demo-data
// seeding logic without rebuilding it.
//
// As of SPEC-013, this package (and everything that imports it) is
// compiled and run as part of the default `make test` / `make check`,
// against a dedicated test database (DB_NAME defaults to "auth_test",
// never the "auth" database used by `make up`). There is no longer an
// `integration` build tag gating DB-backed tests: they are the normal
// tests for this stack.
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

// requireDB reports whether OpenTestDB must fail the test outright
// (via t.Fatal) rather than skip it, given that DB_HOST is unset. It is
// a pure function of the REQUIRE_DB environment variable value so the
// skip-vs-fatal branch selection can be exercised by an ordinary unit
// test without needing to actually terminate a *testing.T.
//
// REQUIRE_DB=1 is injected by the "regular" test paths (CI check jobs,
// the pre-commit DB phase, root-level `make test`) so a misconfigured
// DB_HOST cannot silently skip DB-backed tests and appear green. Ad-hoc
// local `go test ./...` without REQUIRE_DB set keeps the previous
// graceful-skip behavior.
func requireDB(requireDBEnv string) bool {
	return requireDBEnv == "1"
}

// RequireDBHost enforces this package's fail-closed DB_HOST policy for
// tests that need to dial Postgres themselves (e.g. opening more than
// one *sql.DB pool via postgres.OpenPair) instead of going through
// OpenTestDB. It centralizes the same skip-vs-fatal decision OpenTestDB
// applies, so every DB-backed test -- whether or not it uses OpenTestDB
// -- observes REQUIRE_DB=1 the same way:
//
//   - DB_HOST unset and REQUIRE_DB=1: fails the test via t.Fatal (the
//     regular check/CI/pre-commit path must not silently skip
//     DB-backed tests).
//   - DB_HOST unset and REQUIRE_DB unset/other: skips the test, so
//     ad-hoc local `go test ./...` without a live Postgres still
//     compiles and passes.
//   - DB_HOST set: returns normally: the caller may proceed to dial.
func RequireDBHost(t *testing.T) {
	t.Helper()

	if os.Getenv("DB_HOST") != "" {
		return
	}
	if requireDB(os.Getenv("REQUIRE_DB")) {
		t.Fatalf("DB_HOST not set but REQUIRE_DB=1; refusing to silently skip a DB-backed test (see .claude/rules/db.md)")
	}
	t.Skip("DB_HOST not set; skipping DB-backed test (set DB_HOST to a reachable, migrated Postgres, or REQUIRE_DB=1 to fail instead of skip)")
}

// OpenTestDB opens a *sql.DB against the Postgres instance described
// by the discrete DB_* environment variables (DB_HOST / DB_PORT /
// DB_NAME / DB_USER / DB_PASSWORD / DB_SSLMODE).
//
// It applies RequireDBHost's fail-closed policy first: when DB_HOST is
// unset, REQUIRE_DB=1 fails the test via t.Fatal, otherwise it skips
// the test so ad-hoc local `go test ./...` without a live Postgres
// still compiles and passes.
//
// DB_NAME defaults to "auth_test" (a dedicated test database, separate
// from the "auth" database used by `make up`/local development, so
// running tests never touches development data; per SPEC-013).
//
// The returned *sql.DB is registered for cleanup via t.Cleanup; callers
// must not call db.Close() themselves.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()

	RequireDBHost(t)

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
// persistence. DB_NAME defaults to "auth_test".
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
		Name:     env("DB_NAME", "auth_test"),
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
