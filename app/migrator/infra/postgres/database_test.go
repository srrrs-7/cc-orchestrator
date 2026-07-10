package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestIsDuplicateDatabase covers the SQLSTATE classification
// ensureDatabase uses to treat a concurrent CREATE DATABASE race as
// idempotent success (SPEC-005 plan §RF.1.2 step 1 / task RB1: "42P04
// を冪等成功に分類する分岐のユニット。実 DB は不要"). Moved from the
// pre-refactor database_test.go's TestIsDuplicateDatabase. This also
// covers the regression this file's package-level doc references: a
// losing concurrent CREATE DATABASE for the same name can surface as
// SQLSTATE 23505 (unique_violation) on pg_database's own name index
// instead of 42P04 (duplicate_database) -- reproduced 5/5 under real
// concurrent load against two migrator invocations racing to create
// the same not-yet-existing database, because Postgres's
// duplicate-name pre-check in createdb() is not atomic with the
// catalog insert. isDuplicateDatabase must classify that specific
// 23505 as a duplicate_database race too, while still not
// misclassifying an unrelated 23505 (e.g. from some other table's
// unique constraint) as "database already exists". This tests the
// pure classification function only; it never opens a real database
// connection.
func TestIsDuplicateDatabase(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error is not a duplicate_database error",
			err:  nil,
			want: false,
		},
		{
			name: "42P04 (duplicate_database) directly is classified as duplicate",
			err:  &pgconn.PgError{Code: pgDuplicateDatabase},
			want: true,
		},
		{
			name: "42P04 wrapped via fmt.Errorf is still recognized (errors.As traverses Unwrap)",
			err:  fmt.Errorf("create database: %w", &pgconn.PgError{Code: pgDuplicateDatabase}),
			want: true,
		},
		{
			name: "23505 unique_violation on pg_database's name index is classified as a duplicate_database race",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "pg_database_datname_index"},
			want: true,
		},
		{
			name: "23505 wrapped via fmt.Errorf with the pg_database constraint name is still recognized",
			err:  fmt.Errorf("create database: %w", &pgconn.PgError{Code: "23505", ConstraintName: "pg_database_datname_index"}),
			want: true,
		},
		{
			name: "23505 unique_violation naming pg_database as the table (constraint name unset) is classified as a duplicate_database race",
			err:  &pgconn.PgError{Code: "23505", TableName: "pg_database"},
			want: true,
		},
		{
			name: "a bare 23505 with no pg_database constraint or table is not classified as duplicate_database (could be an unrelated unique violation)",
			err:  &pgconn.PgError{Code: "23505"},
			want: false,
		},
		{
			name: "23505 on an unrelated table/constraint is not classified as duplicate_database",
			err:  &pgconn.PgError{Code: "23505", TableName: "tasks", ConstraintName: "tasks_pkey"},
			want: false,
		},
		{
			name: "a non-pgconn error is never classified as duplicate_database",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDuplicateDatabase(tc.err); got != tc.want {
				t.Errorf("isDuplicateDatabase(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
