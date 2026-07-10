package migration

// AppRole is a value object for the least-privilege runtime role this
// migrator provisions for a stack's own database (ISSUE-016 R-c):
// e.g. role name "api_app" paired with DatabaseName "api", or
// "auth_app" paired with "auth". It is a completely different Postgres
// principal from the master credentials this migrator itself connects
// with to run goose and to bootstrap the database (infra/postgres.
// EnsureExister): master creates AppRole, synchronizes its password,
// and grants it the minimal access its own database's runtime needs
// (infra/postgres.RoleEnsurer, the RoleProvisioner port's
// implementation) -- master never runs application queries as it.
//
// Like DatabaseName, AppRole can only be constructed via ParseAppRole,
// so any AppRole an infra/postgres implementation receives already has
// a name validated against the same safe-identifier allowlist
// DatabaseName uses (identifier.go's validateIdentifier): CREATE ROLE/
// GRANT/REVOKE's role-name position is exactly as unparameterizable as
// CREATE DATABASE's target name (see database.go's DatabaseName doc
// comment).
type AppRole struct {
	name     string
	database DatabaseName
}

// ParseAppRole validates name against the safe-identifier allowlist
// (identifierPattern) and pairs it with database -- the DatabaseName
// this role exists to connect to and be granted least privilege on.
// database is assumed already validated by its own constructor
// (ParseDatabaseName), so ParseAppRole does not re-validate it.
func ParseAppRole(name string, database DatabaseName) (AppRole, error) {
	if err := validateIdentifier("app role name", name); err != nil {
		return AppRole{}, err
	}
	return AppRole{name: name, database: database}, nil
}

// Name returns the underlying role name (e.g. "api_app").
func (r AppRole) Name() string {
	return r.name
}

// Database returns the database this role connects to and is granted
// least privilege on.
func (r AppRole) Database() DatabaseName {
	return r.database
}

// Quoted double-quotes the role name for splicing into a DDL statement
// whose identifier position cannot be parameterized (CREATE ROLE/
// GRANT/REVOKE), matching DatabaseName.Quoted's rationale exactly.
func (r AppRole) Quoted() string {
	return quoteIdentifier(r.name)
}

// String returns the underlying role name, so AppRole is directly
// usable with fmt's %s/%q/%v verbs in log and error messages (e.g.
// service.Migrate's "ensure app role %q" wrap) without ever needing
// the password AppRole deliberately does not carry to be passed
// alongside it.
func (r AppRole) String() string {
	return r.name
}
