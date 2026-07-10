package migration

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
// Validation itself is shared with AppRole (identifier.go's
// validateIdentifier): CREATE ROLE/GRANT/REVOKE's role-name position
// is exactly as unparameterizable as CREATE DATABASE's target name.
func ParseDatabaseName(name string) (DatabaseName, error) {
	if err := validateIdentifier("database name", name); err != nil {
		return DatabaseName{}, err
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
	return quoteIdentifier(d.value)
}
