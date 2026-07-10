package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
)

// pgDuplicateObject is the Postgres SQLSTATE code for duplicate_object,
// raised when two concurrent CREATE ROLE statements race to create the
// same role name -- the CREATE ROLE analog of ensureDatabase's
// pgDuplicateDatabase handling (database.go's doc comment explains why
// CREATE DATABASE's own pre-existence check is not atomic with its
// catalog insert; CREATE ROLE has the equivalent non-atomic race
// against pg_authid).
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const pgDuplicateObject = "42710"

// RoleEnsurer implements migration.RoleProvisioner against a real
// Postgres instance (ISSUE-016 R-c): idempotently create (or
// password-sync) a least-privilege runtime role, and grant it exactly
// the access its own database's runtime needs --
//
//   - CONNECT to role.Database() only. EnsureAppRole also revokes the
//     default PUBLIC CONNECT privilege on that database, which is what
//     turns "api and auth each live in their own database" from a mere
//     naming convention into an actual permission boundary: a leaked
//     api_app credential cannot even open a connection to the auth
//     database (and vice versa).
//   - USAGE on schema public -- never CREATE (DDL). EnsureAppRole does
//     not grant any DDL privilege.
//   - SELECT/INSERT/UPDATE/DELETE on every table already in schema
//     public, and (via ALTER DEFAULT PRIVILEGES) every table a future
//     "migrator -command up" run creates there.
//   - USAGE/SELECT on every sequence already in schema public, and
//     (via ALTER DEFAULT PRIVILEGES) every sequence a future run
//     creates there.
//
// EnsureAppRole is idempotent (safe to call on every "up" run) and
// safe under concurrent invocations racing to provision the same role,
// mirroring EnsureExister.EnsureExists's concurrency posture (see
// ensureRole's doc comment).
type RoleEnsurer struct {
	cfg Config
}

// NewRoleEnsurer builds a RoleEnsurer that connects using cfg: first
// to cfg.MaintenanceName for the cluster-wide CREATE ROLE/ALTER ROLE/
// GRANT-REVOKE-ON-DATABASE steps, then to role.Database() itself for
// the database-scoped schema/table/sequence GRANTs (those are scoped
// per-database, so they cannot be issued from the maintenance
// connection).
func NewRoleEnsurer(cfg Config) *RoleEnsurer {
	return &RoleEnsurer{cfg: cfg}
}

// EnsureAppRole implements migration.RoleProvisioner.
func (e *RoleEnsurer) EnsureAppRole(ctx context.Context, role migration.AppRole, password string) error {
	if password == "" {
		return errors.New("migrator: EnsureAppRole: password is empty")
	}

	maintenanceDB, err := Open(ctx, e.cfg.DSN(e.cfg.MaintenanceName))
	if err != nil {
		return fmt.Errorf("open maintenance database %q: %w", e.cfg.MaintenanceName, err)
	}
	defer func() {
		if closeErr := maintenanceDB.Close(); closeErr != nil {
			slog.Error("migrator: close maintenance database", "error", closeErr)
		}
	}()

	if err := ensureRole(ctx, maintenanceDB, role, password); err != nil {
		return fmt.Errorf("ensure role %q: %w", role, err)
	}
	if err := restrictDatabaseConnect(ctx, maintenanceDB, role); err != nil {
		return fmt.Errorf("restrict database connect for role %q: %w", role, err)
	}

	targetDB, err := Open(ctx, e.cfg.DSN(role.Database().String()))
	if err != nil {
		return fmt.Errorf("open target database %q: %w", role.Database(), err)
	}
	defer func() {
		if closeErr := targetDB.Close(); closeErr != nil {
			slog.Error("migrator: close target database", "error", closeErr)
		}
	}()

	if err := grantLeastPrivilege(ctx, targetDB, role); err != nil {
		return fmt.Errorf("grant least privilege for role %q: %w", role, err)
	}
	return nil
}

// maxPasswordSyncAttempts and passwordSyncRetryDelay bound
// ensureRole's retry of a concurrently-raced ALTER ROLE ... PASSWORD
// (see isConcurrentCatalogUpdate's doc comment): empirically (two
// migrator invocations racing to provision the same role against a
// throwaway Postgres 16 container, confirmed reproducible) this
// specific race resolves as soon as the other transaction's catalog
// update commits, which is on the order of milliseconds, so a handful
// of short-delay attempts is enough without meaningfully slowing down
// the common (uncontended) case.
const (
	maxPasswordSyncAttempts = 5
	passwordSyncRetryDelay  = 20 * time.Millisecond
)

// ensureRole makes sure role exists as a LOGIN role on the Postgres
// instance db is connected to, creating it if it does not, and always
// synchronizes its password to password (an existing role may have had
// its password rotated -- e.g. app/iac's random_password -- since the
// last "migrator up" run, plan §1.3).
//
// Like ensureDatabase (database.go), a concurrent CREATE ROLE racing
// this one (two migrator invocations bootstrapping the same instance
// at once) must not fail either side: this recognizes SQLSTATE 42710
// (duplicate_object) as a benign race outcome, and falls back to
// re-checking pg_roles for any other CREATE ROLE failure so the
// classification never depends on a single SQLSTATE the current
// Postgres version happens to use.
//
// A second, distinct race can surface on the password-sync step
// itself: two concurrent ALTER ROLE ... PASSWORD statements against
// the same role can make Postgres's shared-catalog (pg_authid) update
// path raise "tuple concurrently updated" (SQLSTATE XX000,
// isConcurrentCatalogUpdate) instead of succeeding outright --
// reproduced empirically running 3 concurrent "migrator up" processes
// against the same not-yet-password-synced role. Unlike the CREATE
// ROLE race, this is not "already done, so treat as success": the
// losing statement's own password-set attempt simply did not apply,
// so it is retried (bounded by maxPasswordSyncAttempts) rather than
// swallowed.
//
// password is never included in any error this function returns
// (only role's identifier and the driver's own error are): the one
// statement that carries it (ALTER ROLE ... PASSWORD) is built via
// quoteLiteral, whose result is spliced directly into the statement
// text rather than ever being formatted into a log or error message.
func ensureRole(ctx context.Context, db *sql.DB, role migration.AppRole, password string) error {
	exists, err := roleExists(ctx, db, role.Name())
	if err != nil {
		return fmt.Errorf("check existence: %w", err)
	}
	if !exists {
		if _, err := db.ExecContext(ctx, "CREATE ROLE "+role.Quoted()+" LOGIN"); err != nil {
			if !isDuplicateRole(err) {
				// Fall back to re-checking existence regardless of
				// err's SQLSTATE, matching ensureDatabase's own
				// fallback: this is what makes ensureRole robust to a
				// concurrent-creation race surfacing under a code
				// isDuplicateRole does not (yet, or ever) recognize.
				if reExists, reErr := roleExists(ctx, db, role.Name()); reErr != nil || !reExists {
					return fmt.Errorf("create: %w", err)
				}
				// else: created concurrently by another invocation;
				// idempotent success.
			}
		}
	}

	// ALTER ROLE ... PASSWORD's grammar requires a literal string
	// (Sconst) in this position, not a bind parameter -- a
	// database/sql driver's extended-protocol $1 substitution is
	// rejected there -- so password is escaped via quoteLiteral rather
	// than passed as a query argument (see quoteLiteral's doc
	// comment). The statement text (which embeds the escaped password)
	// is never itself included in any error below.
	//nolint:gosec // G202: role is a migration.AppRole, only ever constructed via the allowlist-validated ParseAppRole (identifier.go) and Quoted() additionally escapes it; password is not string-concatenated in raw form but via quoteLiteral, which SQL-literal-escapes it (single quotes doubled / E'' + doubled backslashes) before splicing.
	stmt := "ALTER ROLE " + role.Quoted() + " WITH PASSWORD " + quoteLiteral(password)
	var syncErr error
	for attempt := 0; attempt < maxPasswordSyncAttempts; attempt++ {
		if _, syncErr = db.ExecContext(ctx, stmt); syncErr == nil {
			return nil
		}
		if !isConcurrentCatalogUpdate(syncErr) {
			return fmt.Errorf("set password: %w", syncErr)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("set password: %w", ctx.Err())
		case <-time.After(passwordSyncRetryDelay):
		}
	}
	return fmt.Errorf("set password: exceeded %d attempts under concurrent catalog updates: %w", maxPasswordSyncAttempts, syncErr)
}

// restrictDatabaseConnect revokes the default PUBLIC CONNECT privilege
// on role.Database() and grants CONNECT to role alone. This is the
// statement pair that promotes "api and auth each have their own
// database" from a naming convention into an actual permission
// boundary (ISSUE-016 R-c / plan §1.1): without it, every role on the
// instance -- including, say, auth_app -- can connect to the api
// database by default (Postgres's own default: CONNECT is granted to
// PUBLIC on every database unless explicitly revoked).
//
// Both statements are idempotent: revoking a privilege PUBLIC no
// longer holds is a no-op, and re-granting CONNECT to a role that
// already has it is likewise a no-op, so repeated "migrator up" runs
// are safe.
func restrictDatabaseConnect(ctx context.Context, db *sql.DB, role migration.AppRole) error {
	dbName := role.Database()
	if _, err := db.ExecContext(ctx, "REVOKE CONNECT ON DATABASE "+dbName.Quoted()+" FROM PUBLIC"); err != nil {
		return fmt.Errorf("revoke public connect on database %q: %w", dbName, err)
	}
	if _, err := db.ExecContext(ctx, "GRANT CONNECT ON DATABASE "+dbName.Quoted()+" TO "+role.Quoted()); err != nil {
		return fmt.Errorf("grant connect on database %q: %w", dbName, err)
	}
	return nil
}

// leastPrivilegeStatements returns, in the order they must be applied,
// exactly the SQL statements that grant role its least-privilege
// access on schema public: USAGE on the schema itself, DML on every
// table and sequence already there, and (via ALTER DEFAULT PRIVILEGES)
// the same DML on every table/sequence a future "migrator up" run
// creates -- deliberately never CREATE (DDL) on schema public. This is
// a pure function (no I/O) precisely so ISSUE-016 R-c's central
// requirement -- "these statements never grant DDL" -- can be asserted
// directly against the statement text in a unit test
// (TestGrantLeastPrivilege_NeverGrantsDDL), independent of any real
// Postgres connection.
//
// ALTER DEFAULT PRIVILEGES with no FOR ROLE clause applies to objects
// the *executing* role creates in the future. Because this migrator
// always runs both goose (infra/goose.Runner) and grantLeastPrivilege
// with the same master credentials (cfg.User), "the executing role"
// here is exactly the role that creates every table goose migrates in,
// so no FOR ROLE clause is needed here (docs/plans/ISSUE-016-plan.md
// §5 records this assumption and the risk if migrator's own
// credentials are ever separated from goose's in the future).
func leastPrivilegeStatements(role migration.AppRole) []string {
	return []string{
		"GRANT USAGE ON SCHEMA public TO " + role.Quoted(),
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + role.Quoted(),
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + role.Quoted(),
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO " + role.Quoted(),
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO " + role.Quoted(),
	}
}

// grantLeastPrivilege applies leastPrivilegeStatements(role) against
// db, which must already be connected to role.Database() (schema
// public in one database is a distinct object from schema public in
// another, so these GRANTs cannot be issued from any other
// connection). Every statement here carries no secret, so (unlike
// ensureRole's password step) including the full statement text in
// the returned error is safe and useful for troubleshooting.
func grantLeastPrivilege(ctx context.Context, db *sql.DB, role migration.AppRole) error {
	for _, stmt := range leastPrivilegeStatements(role) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("execute %q: %w", stmt, err)
		}
	}
	return nil
}

// roleExists reports whether a role named name is registered in the
// pg_roles catalog on the instance db is connected to.
func roleExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM pg_roles WHERE rolname = $1", name).Scan(&exists)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	default:
		return false, err
	}
}

// isDuplicateRole reports whether err is a Postgres error indicating a
// concurrent CREATE ROLE already created (or is concurrently creating)
// the same role name: SQLSTATE 42710 (duplicate_object). Mirrors
// isDuplicateDatabase (database.go) for the CREATE ROLE case.
func isDuplicateRole(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == pgDuplicateObject
}

// pgInternalError is the Postgres SQLSTATE code for internal_error.
// It is not specific to any one condition -- Postgres uses it as a
// catch-all for a handful of low-level catalog races -- which is why
// isConcurrentCatalogUpdate additionally checks the error message text
// for the one such race ensureRole's password-sync retry cares about,
// rather than treating every XX000 as retryable.
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const pgInternalError = "XX000"

// isConcurrentCatalogUpdate reports whether err is the "tuple
// concurrently updated" error Postgres raises when two sessions race
// to update the same shared-catalog row (here, pg_authid's row for the
// role ensureRole's ALTER ROLE ... PASSWORD statement targets) --
// confirmed reproducible by running several concurrent "migrator up"
// processes against the same not-yet-synced role, all issuing ALTER
// ROLE ... PASSWORD at once. This has no SQLSTATE of its own (Postgres
// raises it under the generic ERRCODE_INTERNAL_ERROR, pgInternalError),
// so the check also matches on the message Postgres uses for this
// specific race, to avoid treating an unrelated XX000 internal error
// as safe to blindly retry.
func isConcurrentCatalogUpdate(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == pgInternalError && strings.Contains(pgErr.Message, "tuple concurrently updated")
}

// quoteLiteral escapes s for splicing into the one SQL string-literal
// position this package cannot parameterize: ALTER ROLE ... PASSWORD's
// grammar requires a literal Sconst there, not a bind parameter (a
// database/sql driver's extended-protocol $1 substitution is rejected
// by Postgres for this position). This mirrors lib/pq's well-
// established QuoteLiteral helper rather than adding a dependency on
// it (.claude/rules/db.md: this migrator's only new runtime dependency
// is pgx): single quotes are doubled per the SQL standard, and if s
// contains a backslash, the backslashes are additionally doubled and
// the whole literal is prefixed with E, so the value round-trips
// correctly even against a server configured with
// standard_conforming_strings=off (RDS Postgres 16 defaults it on;
// handling the off case too is defense in depth, not a load-bearing
// assumption).
// https://www.postgresql.org/docs/current/sql-syntax-lexical.html#SQL-SYNTAX-STRINGS-ESCAPE
func quoteLiteral(s string) string {
	s = strings.ReplaceAll(s, `'`, `''`)
	if strings.Contains(s, `\`) {
		s = strings.ReplaceAll(s, `\`, `\\`)
		return `E'` + s + `'`
	}
	return `'` + s + `'`
}
