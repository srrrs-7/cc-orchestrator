package migration

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// postgresIdentifierMaxLen is Postgres's NAMEDATALEN limit (64) minus
// one, i.e. the longest identifier Postgres accepts without silently
// truncating it.
// https://www.postgresql.org/docs/current/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
const postgresIdentifierMaxLen = 63

// identifierPattern is the safe subset of Postgres identifiers this
// migrator accepts for a *database* name or an *app role* name:
// lowercase ASCII letters, digits, and underscores, starting with a
// letter or underscore. CREATE DATABASE/CREATE ROLE/GRANT/REVOKE
// cannot parameterize their identifier position the way sqlc-generated
// queries parameterize ordinary DML (.claude/rules/db.md "セキュリ
// ティ": "文字列連結でクエリを組み立てない"), so this migrator instead
// allowlist-validates the name at construction time (ParseDatabaseName
// / ParseAppRole) before any infra layer ever splices it into a
// statement, and always quotes it (Quoted) as defense in depth.
var identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// validateIdentifier rejects any identifier that is not a safe
// Postgres identifier: empty, longer than Postgres allows, containing
// anything outside [a-z0-9_], or not starting with a letter/
// underscore. In particular this excludes quotes, backslashes,
// semicolons, and whitespace of any kind. kind names what is being
// validated (e.g. "database name", "app role name") purely to shape
// the resulting error message; it never affects the validation logic
// itself.
func validateIdentifier(kind, name string) error {
	if name == "" {
		return errors.New("migrator: " + kind + " is empty")
	}
	if len(name) > postgresIdentifierMaxLen {
		return fmt.Errorf("migrator: %s %q exceeds %d bytes", kind, name, postgresIdentifierMaxLen)
	}
	if !identifierPattern.MatchString(name) {
		return fmt.Errorf("migrator: %s %q must match %s", kind, name, identifierPattern.String())
	}
	return nil
}

// quoteIdentifier double-quotes name for splicing into a DDL statement
// whose identifier position cannot be parameterized. It also escapes
// any embedded double quote (Postgres's standard identifier-quoting
// rule, `"` -> `""`) as defense in depth, even though
// validateIdentifier already excludes that character before
// quoteIdentifier is ever reached via DatabaseName.Quoted/
// AppRole.Quoted.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
