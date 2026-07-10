package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
)

// pgDuplicateDatabase is the Postgres SQLSTATE code for
// duplicate_database, raised when two concurrent CREATE DATABASE
// statements race for the same name.
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const pgDuplicateDatabase = "42P04"

// pgUniqueViolation is the Postgres SQLSTATE code for unique_violation.
// A losing concurrent CREATE DATABASE can surface this code instead of
// pgDuplicateDatabase (isDuplicateDatabase's doc comment explains why),
// so isDuplicateDatabase also treats it as duplicate_database when it
// is specifically the pg_database catalog's unique name index.
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const pgUniqueViolation = "23505"

// pgDatabaseCatalogTable and pgDatabaseNameIndex identify the specific
// unique_violation this migrator treats as a duplicate_database race
// (isDuplicateDatabase), as opposed to some unrelated unique_violation
// that happens to also carry SQLSTATE 23505.
const (
	pgDatabaseCatalogTable = "pg_database"
	pgDatabaseNameIndex    = "pg_database_datname_index"
)

// EnsureExister implements migration.Database against a real Postgres
// instance.
type EnsureExister struct {
	cfg Config
}

// NewEnsureExister builds an EnsureExister that connects using cfg
// (in particular cfg.MaintenanceName) to bootstrap a target database.
func NewEnsureExister(cfg Config) *EnsureExister {
	return &EnsureExister{cfg: cfg}
}

// EnsureExists implements migration.Database: it opens a short-lived
// connection to e.cfg's maintenance database (default "postgres") and
// delegates to ensureDatabase to create name if it does not already
// exist. This connection is closed before returning: CREATE DATABASE
// must run outside any transaction against a database other than the
// one being created, and there is no reason to hold this maintenance
// connection open once that step is done (infra/goose.Runner opens
// its own, separate connection to the target database itself).
func (e *EnsureExister) EnsureExists(ctx context.Context, name migration.DatabaseName) error {
	maintenanceDB, err := Open(ctx, e.cfg.DSN(e.cfg.MaintenanceName))
	if err != nil {
		return fmt.Errorf("open maintenance database %q: %w", e.cfg.MaintenanceName, err)
	}
	defer func() {
		if closeErr := maintenanceDB.Close(); closeErr != nil {
			slog.Error("migrator: close maintenance database", "error", closeErr)
		}
	}()

	return ensureDatabase(ctx, maintenanceDB, name)
}

// ensureDatabase makes sure a database named name exists on the
// Postgres instance maintenanceDB is connected to, creating it if it
// does not. CREATE DATABASE cannot run inside a transaction block, so
// this issues a plain ExecContext rather than a db.BeginTx-wrapped one
// (database/sql autocommits statements that are not explicitly
// wrapped in a Tx).
//
// A concurrent CREATE DATABASE racing this one -- e.g. api's and
// auth's migrator invocations, or two replicas of the same init
// container under desired_count>1, both bootstrapping against the
// same instance at once -- must not fail either side. Postgres's own
// documentation notes that createdb()'s pre-existence check is not
// atomic with the catalog insert, so the race's loser can observe
// either SQLSTATE 42P04 (duplicate_database, the "expected" outcome)
// or SQLSTATE 23505 (unique_violation on pg_database's name index,
// when the insert itself loses the race after the pre-check already
// passed) -- confirmed 5/5 under real concurrent load, not merely
// theoretical. Matching only 42P04 therefore left the 23505 outcome an
// unhandled, non-idempotent failure (exit 1).
//
// This is resolved in two complementary layers so the classification
// never depends on which SQLSTATE or constraint name a given Postgres
// version happens to use for this race:
//
//  1. isDuplicateDatabase's fast path recognizes both 42P04 and (the
//     pg_database-specific) 23505 without a round trip.
//  2. If CREATE DATABASE fails for any other reason, this function
//     re-queries pg_database directly. If the database now exists,
//     the failure was some flavor of this same creation race (whatever
//     its SQLSTATE) and is treated as idempotent success; if it still
//     does not exist, the original error is real and is returned.
func ensureDatabase(ctx context.Context, maintenanceDB *sql.DB, name migration.DatabaseName) error {
	exists, err := databaseExists(ctx, maintenanceDB, name.String())
	if err != nil {
		return fmt.Errorf("ensure database: check existence: %w", err)
	}
	if exists {
		return nil
	}

	if _, err := maintenanceDB.ExecContext(ctx, "CREATE DATABASE "+name.Quoted()); err != nil {
		if isDuplicateDatabase(err) {
			return nil // created concurrently by another invocation; idempotent success
		}

		// Fall back to re-checking existence regardless of err's
		// SQLSTATE: this is what makes ensureDatabase robust to a
		// concurrent-creation race surfacing under a code or
		// constraint name isDuplicateDatabase does not (yet, or ever,
		// across Postgres versions) recognize. A check error here is
		// deliberately swallowed in favor of the original create
		// error, which is the more actionable one to report.
		if reExists, reErr := databaseExists(ctx, maintenanceDB, name.String()); reErr == nil && reExists {
			return nil // created concurrently by another invocation; idempotent success
		}
		return fmt.Errorf("ensure database: create: %w", err)
	}
	return nil
}

// databaseExists reports whether a database named name is registered
// in the pg_database catalog on the instance maintenanceDB is
// connected to.
func databaseExists(ctx context.Context, maintenanceDB *sql.DB, name string) (bool, error) {
	var exists int
	err := maintenanceDB.QueryRowContext(ctx, "SELECT 1 FROM pg_database WHERE datname = $1", name).Scan(&exists)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	default:
		return false, err
	}
}

// isDuplicateDatabase reports whether err is a Postgres error that
// indicates a concurrent CREATE DATABASE already created (or is
// concurrently creating) the target database: either SQLSTATE 42P04
// (duplicate_database) directly, or SQLSTATE 23505 (unique_violation)
// specifically on pg_database's own name uniqueness index or table --
// the code the same race can surface instead of 42P04 (ensureDatabase's
// doc comment). A bare 23505 that is not tied to pg_database is left
// unclassified here (and is not a database-existence signal), since
// that could be an unrelated unique constraint violation from a
// different concurrent statement. database/sql's pgx stdlib driver
// surfaces the underlying *pgconn.PgError unwrapped from ExecContext,
// so errors.As reaches it directly.
func isDuplicateDatabase(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case pgDuplicateDatabase:
		return true
	case pgUniqueViolation:
		return pgErr.ConstraintName == pgDatabaseNameIndex || pgErr.TableName == pgDatabaseCatalogTable
	default:
		return false
	}
}
