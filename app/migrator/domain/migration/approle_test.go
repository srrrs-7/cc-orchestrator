package migration

import (
	"strings"
	"testing"
)

// mustDatabaseName is a small test helper: build a DatabaseName from a
// literal known to be valid, failing the test immediately if it is
// not (a bug in the test itself, not the code under test).
func mustDatabaseName(t *testing.T, name string) DatabaseName {
	t.Helper()
	dbName, err := ParseDatabaseName(name)
	if err != nil {
		t.Fatalf("ParseDatabaseName(%q) unexpected error: %v", name, err)
	}
	return dbName
}

// TestParseAppRole covers ParseAppRole's identifier allowlist
// (ISSUE-016 R-c): the same safe subset ParseDatabaseName enforces
// (identifier.go's validateIdentifier/identifierPattern), since
// CREATE ROLE/GRANT/REVOKE's role-name position is exactly as
// unparameterizable as CREATE DATABASE's target name. Covers 正常系
// (real role names this migrator provisions), 異常系 (injection-shaped
// input), and 境界 (length/leading-character edges).
func TestParseAppRole(t *testing.T) {
	db := mustDatabaseName(t, "api")

	cases := []struct {
		name    string
		roleID  string
		wantErr bool
	}{
		// 正常系: the two real role names ISSUE-016 provisions.
		{name: "api_app", roleID: "api_app", wantErr: false},
		{name: "auth_app", roleID: "auth_app", wantErr: false},
		{name: "single lowercase letter", roleID: "a", wantErr: false},
		{name: "leading underscore", roleID: "_svc", wantErr: false},

		// 異常系: empty, SQL metacharacters, quotes, whitespace, casing.
		{name: "empty string", roleID: "", wantErr: true},
		{name: "uppercase letters are rejected", roleID: "API_APP", wantErr: true},
		{name: "embedded semicolon (statement injection shape)", roleID: "api_app; drop role x", wantErr: true},
		{name: "embedded single quote", roleID: "api_app'; select 1 --", wantErr: true},
		{name: "embedded double quote", roleID: `api_app"role`, wantErr: true},
		{name: "embedded space", roleID: "api app", wantErr: true},
		{name: "embedded dash", roleID: "api-app", wantErr: true},

		// 境界: leading digit, exact max length, one over max length.
		{name: "leading digit is rejected", roleID: "1api_app", wantErr: true},
		{name: "exactly 63 bytes (the max) is accepted", roleID: strings.Repeat("a", 63), wantErr: false},
		{name: "64 bytes (one over the max) is rejected", roleID: strings.Repeat("a", 64), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, err := ParseAppRole(tc.roleID, db)
			if tc.wantErr && err == nil {
				t.Errorf("ParseAppRole(%q) = %v, want an error", tc.roleID, role)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ParseAppRole(%q) unexpected error: %v", tc.roleID, err)
			}
			if !tc.wantErr && role.Name() != tc.roleID {
				t.Errorf("ParseAppRole(%q).Name() = %q, want %q", tc.roleID, role.Name(), tc.roleID)
			}
		})
	}
}

// TestAppRole_Database asserts ParseAppRole pairs the role with
// exactly the DatabaseName it was given, unmodified -- the accessor
// infra/postgres.RoleEnsurer relies on to know which database to
// REVOKE/GRANT CONNECT on and to open its schema-scoped GRANT
// connection against.
func TestAppRole_Database(t *testing.T) {
	db := mustDatabaseName(t, "auth")
	role, err := ParseAppRole("auth_app", db)
	if err != nil {
		t.Fatalf("ParseAppRole() unexpected error: %v", err)
	}
	if got := role.Database().String(); got != "auth" {
		t.Errorf("role.Database().String() = %q, want %q", got, "auth")
	}
}

// TestAppRole_Quoted mirrors TestDatabaseName_Quoted (database_test.go):
// a plain role name gets wrapped in double quotes, and any embedded
// double quote (which ParseAppRole itself would reject, but Quoted is
// tested independently here per DatabaseName.Quoted's own precedent)
// is escaped by doubling it.
func TestAppRole_Quoted(t *testing.T) {
	db := mustDatabaseName(t, "api")
	cases := []struct {
		name string
		role string
		want string
	}{
		{name: "plain role name", role: "api_app", want: `"api_app"`},
		{name: "embedded double quote is escaped by doubling", role: `a"b`, want: `"a""b"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AppRole{name: tc.role, database: db}.Quoted()
			if got != tc.want {
				t.Errorf("AppRole{%q}.Quoted() = %q, want %q", tc.role, got, tc.want)
			}
		})
	}
}

// TestAppRole_String asserts String() (used by %q/%s in
// service.Migrate's and infra/postgres.RoleEnsurer's error-wrapping)
// returns the bare role name, never the password AppRole deliberately
// does not carry.
func TestAppRole_String(t *testing.T) {
	db := mustDatabaseName(t, "api")
	role, err := ParseAppRole("api_app", db)
	if err != nil {
		t.Fatalf("ParseAppRole() unexpected error: %v", err)
	}
	if got := role.String(); got != "api_app" {
		t.Errorf("role.String() = %q, want %q", got, "api_app")
	}
}
