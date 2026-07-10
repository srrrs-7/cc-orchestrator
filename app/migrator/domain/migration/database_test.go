package migration

import (
	"strings"
	"testing"
)

// TestParseDatabaseName covers the allowlist infra/postgres relies on
// (via the already-validated DatabaseName it receives) before
// splicing a database name into an unparameterizable CREATE DATABASE
// statement (database.go's identifierPattern doc comment). Moved from
// the pre-refactor database.go's TestValidateIdentifier: normal
// (正常), injection-shaped/invalid (異常) and length/leading-character
// (境界) cases.
func TestParseDatabaseName(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		// 正常系: the two real database names this migrator ever
		// defaults to (cmd/migrator/env.go), plus other legal shapes.
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
			name, err := ParseDatabaseName(tc.id)
			if tc.wantErr && err == nil {
				t.Errorf("ParseDatabaseName(%q) = %v, want an error", tc.id, name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ParseDatabaseName(%q) unexpected error: %v", tc.id, err)
			}
			if !tc.wantErr && name.String() != tc.id {
				t.Errorf("ParseDatabaseName(%q).String() = %q, want %q", tc.id, name.String(), tc.id)
			}
		})
	}
}

// TestDatabaseName_Quoted covers Quoted's own contract
// (defense-in-depth double-quoting, independent of ParseDatabaseName
// having already run): a plain identifier gets wrapped in double
// quotes, and any embedded double quote is escaped by doubling it
// (Postgres's standard identifier-quoting rule).
func TestDatabaseName_Quoted(t *testing.T) {
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
			got := DatabaseName{value: tc.id}.Quoted()
			if got != tc.want {
				t.Errorf("DatabaseName{%q}.Quoted() = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}
