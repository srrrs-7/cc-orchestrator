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
// migrator accepts for a *database* name: lowercase ASCII letters,
// digits, and underscores, starting with a letter or underscore.
// CREATE DATABASE cannot parameterize its target identifier the way
// sqlc-generated queries parameterize ordinary DML (.claude/rules/
// db.md "セキュリティ": "文字列連結でクエリを組み立てない"), so this
// migrator instead allowlist-validates the name at construction time
// (ParseDatabaseName) before any infra layer ever splices it into a
// statement, and always quotes it (Quoted) as defense in depth.
var identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// DatabaseName is a value object for the Postgres database a migrator
// run ensures exists and then migrates. Because it can only be
// constructed via ParseDatabaseName, any DatabaseName value an infra
// implementation (infra/postgres.EnsureExister, infra/goose.Runner)
// receives is already known to be a safe identifier -- the allowlist
// check runs exactly once, here, rather than being re-validated (or
// worse, skipped) at each call site.
type DatabaseName struct {
	value string
}

// ParseDatabaseName rejects any database name that is not a safe
// Postgres identifier: empty, longer than Postgres allows, containing
// anything outside [a-z0-9_], or not starting with a letter/
// underscore. In particular this excludes quotes, backslashes,
// semicolons, and whitespace of any kind -- the injection guard
// CREATE DATABASE's unparameterizable identifier position needs.
func ParseDatabaseName(name string) (DatabaseName, error) {
	if name == "" {
		return DatabaseName{}, errors.New("migrator: database name is empty")
	}
	if len(name) > postgresIdentifierMaxLen {
		return DatabaseName{}, fmt.Errorf("migrator: database name %q exceeds %d bytes", name, postgresIdentifierMaxLen)
	}
	if !identifierPattern.MatchString(name) {
		return DatabaseName{}, fmt.Errorf("migrator: database name %q must match %s", name, identifierPattern.String())
	}
	return DatabaseName{value: name}, nil
}

// String returns the underlying database name.
func (d DatabaseName) String() string {
	return d.value
}

// Quoted double-quotes the database name for splicing into a DDL
// statement whose identifier position cannot be parameterized.
// Quoted also escapes any embedded double quote (Postgres's standard
// identifier-quoting rule, `"` -> `""`) as defense in depth, even
// though identifierPattern already excludes that character.
func (d DatabaseName) Quoted() string {
	return `"` + strings.ReplaceAll(d.value, `"`, `""`) + `"`
}
