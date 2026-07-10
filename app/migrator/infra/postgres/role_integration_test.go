//go:build integration

// Package postgres_test holds ISSUE-016 review Major-1's integration
// suite for app/migrator's infra/postgres.RoleEnsurer (the
// migration.RoleProvisioner port's implementation): confirms the
// least-privilege role boundary (ISSUE-016 R-c) actually holds against
// a real Postgres instance, complementing role_test.go's
// TestGrantLeastPrivilege_NeverGrantsDDL, which only asserts the
// static statement text grantLeastPrivilege builds -- never that
// Postgres itself enforces it.
//
// Gated behind the "integration" build tag (mirrors
// app/{api,auth}/infra/postgres's own integration suites): the default
// `make test` (no build tags, no DB required) stays green. Run
// explicitly, e.g.:
//
//	go test -tags=integration ./infra/postgres/...
//
// against a live, reachable Postgres (master/superuser credentials via
// the discrete DB_* environment variables below -- the same ones
// cmd/migrator/env.go reads at runtime). It is skipped, not failed,
// when DB_HOST is unset, mirroring app/auth/infra/postgres's own
// openTestDB (testdb_integration_test.go) convention, so it does not
// require a live database merely to be present in the tree. CI
// (impl-ci) is expected to wire a postgres service container + these
// DB_* variables for a job that runs this suite, the same way it
// already does for the api-integration/auth-integration jobs.
//
// Unlike app/{api,auth}'s own integration suites (which assume
// `app/migrator -command up` already ran against a pre-existing target
// database), this suite is self-contained: it creates its own
// throwaway databases (via the same EnsureExister this migrator uses
// in production) and drops them again on cleanup, since what is under
// test here is the provisioning step itself (EnsureAppRole), not
// application-level persistence. It requires Postgres 13+ (DROP
// DATABASE ... WITH (FORCE), used only for cleanup); every environment
// this migrator targets (RDS Postgres 16, CI's postgres:17.5-alpine)
// satisfies that.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
	"github.com/srrrs-7/cc-orchestrator/app/migrator/infra/postgres"
)

// masterConfigFromEnv builds the master-credentialed postgres.Config
// this suite uses both to bootstrap its own throwaway databases/roles
// and to assert master's own access is unaffected, from the same
// discrete DB_* environment variables cmd/migrator/env.go reads at
// runtime. It skips the test (t.Skip), not fails it, when DB_HOST is
// unset, matching app/auth's openTestDB convention exactly.
func masterConfigFromEnv(t *testing.T) postgres.Config {
	t.Helper()
	host := os.Getenv("DB_HOST")
	if host == "" {
		t.Skip("DB_HOST not set; skipping app/migrator role-provisioning integration test (ISSUE-016 review Major-1)")
	}
	return postgres.Config{
		Host:            host,
		Port:            envOrDefault("DB_PORT", "5432"),
		User:            envOrDefault("DB_USER", "app"),
		Password:        envOrDefault("DB_PASSWORD", "app"),
		SSLMode:         envOrDefault("DB_SSLMODE", "disable"),
		MaintenanceName: envOrDefault("DB_MAINTENANCE_NAME", "postgres"),
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// uniqueSuffix returns a short, lowercase-alphanumeric, collision-
// resistant suffix (base36 nanosecond timestamp) for this run's
// throwaway database/role names, so repeated local runs against a
// persistent (non-ephemeral) Postgres -- e.g. compose's postgres
// service -- never collide with a still-present database a prior,
// uncleaned run left behind. base36 (0-9, a-z) is itself a safe subset
// of migration.identifierPattern, so the resulting names never need
// any further sanitizing.
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func mustParseDatabaseName(t *testing.T, name string) migration.DatabaseName {
	t.Helper()
	dbName, err := migration.ParseDatabaseName(name)
	if err != nil {
		t.Fatalf("migration.ParseDatabaseName(%q) unexpected error: %v", name, err)
	}
	return dbName
}

func mustParseAppRole(t *testing.T, name string, db migration.DatabaseName) migration.AppRole {
	t.Helper()
	role, err := migration.ParseAppRole(name, db)
	if err != nil {
		t.Fatalf("migration.ParseAppRole(%q): %v", name, err)
	}
	return role
}

// openConn opens a *sql.DB against dbName using cfg's credentials
// (master or a scoped role, depending on cfg.User/cfg.Password) and
// registers its Close for this test's cleanup.
func openConn(t *testing.T, ctx context.Context, cfg postgres.Config, dbName migration.DatabaseName) *sql.DB {
	t.Helper()
	db, err := postgres.Open(ctx, cfg.DSN(dbName.String()))
	if err != nil {
		t.Fatalf("open connection (user %q) to database %q: %v", cfg.User, dbName, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// scopedConfig derives a scoped-role Config from master (same
// Host/Port/SSLMode/MaintenanceName), swapping in the scoped role's
// own credentials -- exactly what app/iac wires into api/auth's
// runtime containers post-ISSUE-016 (docs/plans/ISSUE-016-plan.md
// §1.3), as opposed to master's own DB_USER/DB_PASSWORD.
func scopedConfig(master postgres.Config, user, password string) postgres.Config {
	cfg := master
	cfg.User = user
	cfg.Password = password
	return cfg
}

// dropDatabase best-effort drops name using cfg's maintenance
// connection, logging (not failing the test) on error: this runs
// during t.Cleanup, where a failure to tidy up a throwaway database
// should not mask the test's actual pass/fail result. WITH (FORCE)
// (Postgres 13+) terminates any connections this suite's own subtests
// left open against name, so cleanup never depends on subtest-level
// connection closes having already run first.
func dropDatabase(t *testing.T, cfg postgres.Config, name migration.DatabaseName) {
	t.Helper()
	ctx := context.Background()
	db, err := postgres.Open(ctx, cfg.DSN(cfg.MaintenanceName))
	if err != nil {
		t.Logf("cleanup: open maintenance connection to drop database %q: %v", name, err)
		return
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name.Quoted()+" WITH (FORCE)"); err != nil {
		t.Logf("cleanup: drop database %q: %v", name, err)
	}
}

// dropRole best-effort drops role using cfg's maintenance connection
// (mirrors dropDatabase's best-effort logging contract). Callers MUST
// register this only after the database(s) role was granted CONNECT
// on have already been dropped: a role still holding a "GRANT CONNECT
// ON DATABASE ... TO role" privilege is a shared (cluster-level)
// dependency Postgres refuses to DROP ROLE through ("role ... cannot
// be dropped because some objects depend on it"), and dropping the
// database is what removes that dependency (rather than requiring an
// explicit REVOKE here first).
func dropRole(t *testing.T, cfg postgres.Config, role migration.AppRole) {
	t.Helper()
	ctx := context.Background()
	db, err := postgres.Open(ctx, cfg.DSN(cfg.MaintenanceName))
	if err != nil {
		t.Logf("cleanup: open maintenance connection to drop role %q: %v", role, err)
		return
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "DROP ROLE IF EXISTS "+role.Quoted()); err != nil {
		t.Logf("cleanup: drop role %q: %v", role, err)
	}
}

// pgInsufficientPrivilege is the Postgres SQLSTATE code for
// insufficient_privilege, raised for both a DDL statement a
// least-privilege role lacks CREATE for, and a CONNECT attempt against
// a database whose PUBLIC CONNECT privilege restrictDatabaseConnect
// (role.go) has revoked.
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const pgInsufficientPrivilege = "42501"

// isPermissionDenied reports whether err is the Postgres
// insufficient_privilege error (SQLSTATE 42501) this suite's
// DDL-denied and CONNECT-denied assertions expect. It falls back to a
// case-insensitive "permission denied" substring match for the (rarer)
// case where the driver surfaces a CONNECT-time FATAL as something
// other than an unwrapped *pgconn.PgError, so these assertions do not
// depend on that unwrapping detail.
func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgInsufficientPrivilege
	}
	return strings.Contains(strings.ToLower(err.Error()), "permission denied")
}

// TestRoleEnsurer_LeastPrivilegeBoundary is ISSUE-016 review Major-1's
// CI regression guard for R-c's permission boundary: after
// RoleEnsurer.EnsureAppRole provisions two least-privilege roles (one
// per throwaway database, mirroring api_app/auth_app), each scoped
// role can perform DML (including on a table its own master creates
// *after* EnsureAppRole ran, exercising ALTER DEFAULT PRIVILEGES) but
// never DDL, neither scoped role can CONNECT to the other's database,
// and master retains access to both.
func TestRoleEnsurer_LeastPrivilegeBoundary(t *testing.T) {
	ctx := context.Background()
	masterCfg := masterConfigFromEnv(t)

	suffix := uniqueSuffix()
	dbA := mustParseDatabaseName(t, "role_it_a_"+suffix)
	dbB := mustParseDatabaseName(t, "role_it_b_"+suffix)
	roleA := mustParseAppRole(t, "role_it_ra_"+suffix, dbA)
	roleB := mustParseAppRole(t, "role_it_rb_"+suffix, dbB)
	const passwordA = "role-it-pw-a" //nolint:gosec // G101: throwaway credential for a disposable, test-only role/database, never a real secret.
	const passwordB = "role-it-pw-b" //nolint:gosec // G101: same as passwordA.

	ensurer := postgres.NewEnsureExister(masterCfg)
	if err := ensurer.EnsureExists(ctx, dbA); err != nil {
		t.Fatalf("EnsureExists(%q): %v", dbA, err)
	}
	if err := ensurer.EnsureExists(ctx, dbB); err != nil {
		t.Fatalf("EnsureExists(%q): %v", dbB, err)
	}

	provisioner := postgres.NewRoleEnsurer(masterCfg)
	if err := provisioner.EnsureAppRole(ctx, roleB, passwordB); err != nil {
		t.Fatalf("EnsureAppRole(%q): %v", roleB, err)
	}

	// master creates a pre-existing table in dbA *before* granting, to
	// exercise "GRANT ... ON ALL TABLES" (existing objects) here, and a
	// second table *after* granting, later, to exercise ALTER DEFAULT
	// PRIVILEGES (future objects) separately.
	masterDBA := openConn(t, ctx, masterCfg, dbA)
	if _, err := masterDBA.ExecContext(ctx, "CREATE TABLE t_existing (id serial PRIMARY KEY, name text)"); err != nil {
		t.Fatalf("create pre-existing table: %v", err)
	}

	if err := provisioner.EnsureAppRole(ctx, roleA, passwordA); err != nil {
		t.Fatalf("EnsureAppRole(%q): %v", roleA, err)
	}

	// Cleanup order matters: a role still holding "GRANT CONNECT ON
	// DATABASE ... TO role" cannot be dropped until the database (and
	// that grant with it) is gone, so the role-drop cleanups are
	// registered first here -- t.Cleanup runs last-registered-first, so
	// the database-drop cleanups (registered below) run before these.
	t.Cleanup(func() { dropRole(t, masterCfg, roleA) })
	t.Cleanup(func() { dropRole(t, masterCfg, roleB) })
	t.Cleanup(func() { dropDatabase(t, masterCfg, dbA) })
	t.Cleanup(func() { dropDatabase(t, masterCfg, dbB) })

	scopedA := scopedConfig(masterCfg, roleA.Name(), passwordA)
	scopedB := scopedConfig(masterCfg, roleB.Name(), passwordB)
	scopedADB := openConn(t, ctx, scopedA, dbA)

	t.Run("scoped role can SELECT/INSERT on a table that existed before EnsureAppRole", func(t *testing.T) {
		if _, err := scopedADB.ExecContext(ctx, "INSERT INTO t_existing (name) VALUES ($1)", "hello"); err != nil {
			t.Errorf("INSERT as scoped role: unexpected error: %v", err)
		}
		var count int
		if err := scopedADB.QueryRowContext(ctx, "SELECT count(*) FROM t_existing").Scan(&count); err != nil {
			t.Errorf("SELECT as scoped role: unexpected error: %v", err)
		}
	})

	t.Run("scoped role cannot perform DDL", func(t *testing.T) {
		cases := []struct {
			name string
			stmt string
		}{
			{name: "CREATE TABLE", stmt: "CREATE TABLE t_should_not_exist (id int)"},
			{name: "ALTER TABLE", stmt: "ALTER TABLE t_existing ADD COLUMN extra text"},
			{name: "DROP TABLE", stmt: "DROP TABLE t_existing"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := scopedADB.ExecContext(ctx, tc.stmt)
				if !isPermissionDenied(err) {
					t.Errorf("%s as scoped role = %v, want a permission-denied (42501) error", tc.name, err)
				}
			})
		}
	})

	t.Run("neither scoped role can CONNECT to the other's database", func(t *testing.T) {
		cases := []struct {
			name   string
			cfg    postgres.Config
			target migration.DatabaseName
		}{
			{name: "roleA (dbA) -> dbB", cfg: scopedA, target: dbB},
			{name: "roleB (dbB) -> dbA", cfg: scopedB, target: dbA},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := postgres.Open(ctx, tc.cfg.DSN(tc.target.String()))
				if !isPermissionDenied(err) {
					t.Errorf("Open(%s) = %v, want a permission-denied error", tc.name, err)
				}
			})
		}
	})

	t.Run("master can still connect to both databases", func(t *testing.T) {
		if _, err := postgres.Open(ctx, masterCfg.DSN(dbA.String())); err != nil {
			t.Errorf("master Open(dbA) unexpected error: %v", err)
		}
		if _, err := postgres.Open(ctx, masterCfg.DSN(dbB.String())); err != nil {
			t.Errorf("master Open(dbB) unexpected error: %v", err)
		}
	})

	t.Run("scoped role gets DML on a table master creates after EnsureAppRole (ALTER DEFAULT PRIVILEGES)", func(t *testing.T) {
		if _, err := masterDBA.ExecContext(ctx, "CREATE TABLE t_after (id serial PRIMARY KEY, name text)"); err != nil {
			t.Fatalf("create post-grant table: %v", err)
		}
		if _, err := scopedADB.ExecContext(ctx, "INSERT INTO t_after (name) VALUES ($1)", "world"); err != nil {
			t.Errorf("INSERT into post-grant table as scoped role: unexpected error: %v", err)
		}
	})
}
