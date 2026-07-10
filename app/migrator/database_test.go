package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestValidateIdentifier covers the allowlist ensureDatabase relies on
// before splicing a database name into an unparameterizable CREATE
// DATABASE statement (database.go's identifierPattern doc comment):
// normal (正常), injection-shaped/invalid (異常) and length/leading-
// character (境界) cases.
func TestValidateIdentifier(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		// 正常系: the two real database names this migrator ever
		// defaults to (configFromEnv), plus other legal shapes.
		{name: "api", id: "api", wantErr: false},
		{name: "auth", id: "auth", wantErr: false},
		{name: "single lowercase letter", id: "a", wantErr: false},
		{name: "leading underscore", id: "_bootstrap", wantErr: false},
		{name: "digits and underscores after the first character", id: "a1_2b_3", wantErr: false},

		// 異常系: empty, SQL metacharacters, quotes, whitespace,
		// disallowed casing.
		{name: "empty string", id: "", wantErr: true},
		{name: "uppercase letters are rejected", id: "API", wantErr: true},
		{name: "embedded semicolon (statement injection shape)", id: "api;drop table x", wantErr: true},
		{name: "embedded single quote", id: "api'; select 1 --", wantErr: true},
		{name: "embedded double quote", id: `api"db`, wantErr: true},
		{name: "embedded space", id: "api db", wantErr: true},
		{name: "embedded backslash", id: `api\db`, wantErr: true},
		{name: "embedded dash (not in [a-z0-9_])", id: "api-db", wantErr: true},
		{name: "embedded dot", id: "api.db", wantErr: true},

		// 境界: leading digit, exact max length, one over max length.
		{name: "leading digit is rejected", id: "1api", wantErr: true},
		{name: "exactly 63 bytes (the max) is accepted", id: strings.Repeat("a", 63), wantErr: false},
		{name: "64 bytes (one over the max) is rejected", id: strings.Repeat("a", 64), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIdentifier(tc.id)
			if tc.wantErr && err == nil {
				t.Errorf("validateIdentifier(%q) = nil, want an error", tc.id)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateIdentifier(%q) unexpected error: %v", tc.id, err)
			}
		})
	}
}

// TestQuoteIdentifier covers quoteIdentifier's own contract
// (defense-in-depth double-quoting, independent of validateIdentifier
// having already run): a plain identifier gets wrapped in double
// quotes, and any embedded double quote is escaped by doubling it
// (Postgres's standard identifier-quoting rule), per quoteIdentifier's
// doc comment.
func TestQuoteIdentifier(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want string
	}{
		{name: "plain identifier", id: "api", want: `"api"`},
		{name: "embedded double quote is escaped by doubling", id: `a"b`, want: `"a""b"`},
		{name: "empty string still gets wrapped", id: "", want: `""`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteIdentifier(tc.id)
			if got != tc.want {
				t.Errorf("quoteIdentifier(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

// TestIsDuplicateDatabase covers the SQLSTATE classification
// ensureDatabase uses to treat a concurrent CREATE DATABASE race as
// idempotent success (plan §RF.1.2 step 1 / SPEC-005 task RB1: "42P04
// を冪等成功に分類する分岐のユニット。実 DB は不要"). This also covers
// the regression this test file's package-level doc references: a
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
