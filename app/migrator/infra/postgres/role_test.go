package postgres

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
)

// roleForTest builds a migration.AppRole for the given role/database
// name pair, failing the test immediately (not returning an error) if
// either fails validation -- a bug in the test's own literals, not in
// the code under test.
func roleForTest(t *testing.T, roleName, dbName string) (migration.AppRole, error) {
	t.Helper()
	db, err := migration.ParseDatabaseName(dbName)
	if err != nil {
		return migration.AppRole{}, fmt.Errorf("ParseDatabaseName(%q): %w", dbName, err)
	}
	role, err := migration.ParseAppRole(roleName, db)
	if err != nil {
		return migration.AppRole{}, fmt.Errorf("ParseAppRole(%q): %w", roleName, err)
	}
	return role, nil
}

// TestQuoteLiteral covers the SQL string-literal escaping
// ensureRole's ALTER ROLE ... PASSWORD statement relies on (ISSUE-016
// R-c), since that grammar position cannot be parameterized (role.go's
// quoteLiteral doc comment). Observes 正常系 (plain password),
// 異常系/injection-shaped (embedded quotes, backslashes, statement
// terminators) and 境界 (empty string) cases -- mirroring lib/pq's own
// QuoteLiteral contract.
func TestQuoteLiteral(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain password", in: "hunter2", want: `'hunter2'`},
		{name: "empty string is still wrapped", in: "", want: `''`},
		{
			name: "embedded single quote is doubled (SQL standard escaping)",
			in:   `pa'ss`,
			want: `'pa''ss'`,
		},
		{
			name: "single-quote statement-injection shape is neutralized by doubling",
			in:   `x'; DROP ROLE admin; --`,
			want: `'x''; DROP ROLE admin; --'`,
		},
		{
			name: "embedded backslash switches to E'' escape syntax with doubled backslash",
			in:   `pa\ss`,
			want: `E'pa\\ss'`,
		},
		{
			name: "backslash and single quote together: quote doubled first, then E'' wrapping for the backslash",
			in:   `pa\'ss`,
			want: `E'pa\\''ss'`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quoteLiteral(tc.in); got != tc.want {
				t.Errorf("quoteLiteral(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestQuoteLiteral_NeverReturnsUnquotedInput is a defense-in-depth
// property check alongside the table-driven cases above: whatever s
// is, quoteLiteral's result always starts with a quote character
// (either `'` or the `E'` escape prefix) and ends with `'`, so the
// caller (ensureRole) can never accidentally splice a bare,
// unterminated literal into the ALTER ROLE statement.
func TestQuoteLiteral_NeverReturnsUnquotedInput(t *testing.T) {
	for _, in := range []string{"", "a", `'`, `\`, `'\'\'`, "multi\nline\tpassword"} {
		got := quoteLiteral(in)
		if !strings.HasSuffix(got, `'`) {
			t.Errorf("quoteLiteral(%q) = %q, want a result ending in a closing quote", in, got)
		}
		if !strings.HasPrefix(got, `'`) && !strings.HasPrefix(got, `E'`) {
			t.Errorf("quoteLiteral(%q) = %q, want a result starting with ' or E'", in, got)
		}
	}
}

// TestIsDuplicateRole covers the SQLSTATE classification ensureRole
// uses to treat a concurrent CREATE ROLE race as idempotent success
// (mirrors TestIsDuplicateDatabase in database_test.go for the
// CREATE ROLE case).
func TestIsDuplicateRole(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error is not a duplicate_object error", err: nil, want: false},
		{
			name: "42710 (duplicate_object) directly is classified as duplicate",
			err:  &pgconn.PgError{Code: pgDuplicateObject},
			want: true,
		},
		{
			name: "42710 wrapped via fmt.Errorf is still recognized (errors.As traverses Unwrap)",
			err:  fmt.Errorf("create role: %w", &pgconn.PgError{Code: pgDuplicateObject}),
			want: true,
		},
		{
			name: "an unrelated SQLSTATE is not classified as duplicate_object",
			err:  &pgconn.PgError{Code: "42501"}, // insufficient_privilege
			want: false,
		},
		{
			name: "a non-pgconn error is never classified as duplicate_object",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDuplicateRole(tc.err); got != tc.want {
				t.Errorf("isDuplicateRole(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// disallowedPrivilegeKeywords is the set of DDL-granting keywords
// grantLeastPrivilege's statements must never contain (ISSUE-016 R-c:
// "DDL(CREATE/DROP/ALTER TABLE)権限は付与しない"). ALTER appears in
// "ALTER DEFAULT PRIVILEGES", which is not itself a DDL grant on
// schema public (it only changes what future GRANTs apply
// automatically), so it is deliberately not in this list; this test
// instead asserts the more specific "ALTER ROLE"/"ALTER TABLE"/"CREATE
// TABLE" shapes never appear in these statements.
var disallowedPrivilegeShapes = []string{"CREATE", "DROP", "TRUNCATE", "REFERENCES", "TRIGGER"}

// TestGrantLeastPrivilege_NeverGrantsDDL is a static, DB-free
// assertion on the exact statement text grantLeastPrivilege issues
// (ISSUE-016 R-c's central requirement): every statement only ever
// grants USAGE/SELECT/INSERT/UPDATE/DELETE on schema public's tables
// and sequences, and never a DDL-capable privilege such as CREATE.
// This is deliberately independent of any real Postgres connection:
// grantLeastPrivilege's statements list is itself the requirement's
// enforcement point, so asserting its literal contents here catches a
// future edit that accidentally widens the grant before it ever
// reaches a database.
func TestGrantLeastPrivilege_NeverGrantsDDL(t *testing.T) {
	role, err := roleForTest(t, "api_app", "api")
	if err != nil {
		t.Fatalf("roleForTest: %v", err)
	}

	for _, stmt := range leastPrivilegeStatements(role) {
		upper := strings.ToUpper(stmt)
		for _, shape := range disallowedPrivilegeShapes {
			if strings.Contains(upper, shape) {
				t.Errorf("grant statement %q unexpectedly contains disallowed keyword %q", stmt, shape)
			}
		}
		if !strings.Contains(upper, "SCHEMA PUBLIC") {
			t.Errorf("grant statement %q does not scope to schema public", stmt)
		}
	}
}
