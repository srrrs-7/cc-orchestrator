package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
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

// postgresIdentifierMaxLen is Postgres's NAMEDATALEN limit (64) minus
// one, i.e. the longest identifier Postgres accepts without silently
// truncating it.
// https://www.postgresql.org/docs/current/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
const postgresIdentifierMaxLen = 63

// identifierPattern is the safe subset of Postgres identifiers this
// migrator accepts for a *database* name: lowercase ASCII letters,
// digits, and underscores, starting with a letter or underscore.
// CREATE DATABASE cannot parameterize its target identifier the way
// sqlc-generated queries parameterize ordinary DML (.claude/rules/db.md
// "セキュリティ": "文字列連結でクエリを組み立てない"), so this migrator
// instead allowlist-validates the name (validateIdentifier) before
// splicing it into the statement, and always quotes it
// (quoteIdentifier) as defense in depth.
var identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// validateIdentifier rejects any database name that is not a safe
// Postgres identifier: empty, longer than Postgres allows, containing
// anything outside [a-z0-9_], or not starting with a letter/
// underscore. In particular this excludes quotes, backslashes,
// semicolons, and whitespace of any kind -- the injection guard
// CREATE DATABASE's unparameterizable identifier position needs.
func validateIdentifier(name string) error {
	if name == "" {
		return errors.New("database name is empty")
	}
	if len(name) > postgresIdentifierMaxLen {
		return fmt.Errorf("database name %q exceeds %d bytes", name, postgresIdentifierMaxLen)
	}
	if !identifierPattern.MatchString(name) {
		return fmt.Errorf("database name %q must match %s", name, identifierPattern.String())
	}
	return nil
}

// quoteIdentifier double-quotes name for splicing into a DDL statement
// whose identifier position cannot be parameterized. Callers MUST call
// validateIdentifier first; quoteIdentifier also escapes any embedded
// double quote (Postgres's standard identifier-quoting rule, `"` ->
// `""`) as defense in depth, even though identifierPattern already
// excludes that character.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// ensureDatabase makes sure a database named name exists on the
// Postgres instance maintenanceDB is connected to, creating it if it
// does not (plan §RF.1.2 step 1). CREATE DATABASE cannot run inside a
// transaction block, so this issues a plain ExecContext rather than a
// db.BeginTx-wrapped one (database/sql autocommits statements that are
// not explicitly wrapped in a Tx).
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
func ensureDatabase(ctx context.Context, maintenanceDB *sql.DB, name string) error {
	if err := validateIdentifier(name); err != nil {
		return fmt.Errorf("ensure database: %w", err)
	}

	exists, err := databaseExists(ctx, maintenanceDB, name)
	if err != nil {
		return fmt.Errorf("ensure database: check existence: %w", err)
	}
	if exists {
		return nil
	}

	if _, err := maintenanceDB.ExecContext(ctx, "CREATE DATABASE "+quoteIdentifier(name)); err != nil {
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
		if reExists, reErr := databaseExists(ctx, maintenanceDB, name); reErr == nil && reExists {
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
// so errors.As reaches it directly (same pattern as
// app/api/infra/postgres/task_repository.go's isUniqueViolation).
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
