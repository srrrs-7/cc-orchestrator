// This file is the single place app/migrator reads the process
// environment. No other file in this module should call os.Getenv;
// everything else receives already-captured values via Env,
// postgres.Config, or the migration domain's value objects (mirrors
// app/api/cmd/api/env.go's "single place" doc comment, and
// app/auth/cmd/authz/env.go's).
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
	"github.com/srrrs-7/cc-orchestrator/app/migrator/infra/postgres"
)

// Defaults for the DB_* settings that are safe to default. DB_HOST/
// DB_USER/DB_PASSWORD have no default and are validated as required by
// Env.validate, matching app/{api,auth}/infra/postgres.Config.Validate's
// existing contract for those three variables.
//
// defaultSSLMode is fail-closed ("require"): if DB_SSLMODE is left
// unset, the connection defaults to encrypted rather than silently
// falling back to a plaintext one (ISSUE-016 m-2). This matters most
// here, since this migrator connects with the master credentials to
// run CREATE DATABASE / goose migrations -- the highest-privilege
// connection in the stack. Local development against a non-TLS
// Postgres (e.g. compose's postgres service) must set
// DB_SSLMODE=disable explicitly (see the root Makefile's
// MIGRATOR_DB_ENV, which already does so).
//
// defaultMigrationTimeout bounds how long the goose migration run
// itself (infra/goose.Runner.Run) may take, overridable via
// MIGRATOR_TIMEOUT (see migratorTimeout).
const (
	defaultDBPort           = "5432"
	defaultSSLMode          = "require"
	defaultMaintenanceName  = "postgres"
	defaultMigrationTimeout = 5 * time.Minute
)

// Env holds every environment-derived value app/migrator needs at
// startup: the discrete DB_* connection settings (mirroring
// app/{api,auth}/cmd/*/env.go's contract), this module's own
// MIGRATOR_TIMEOUT override, and the optional APP_DB_USER/
// APP_DB_PASSWORD pair (ISSUE-016 R-c) naming the least-privilege
// runtime role this migrator should provision after a successful "up"
// migration.
//
// DBName is left unresolved against -target here (it may be empty):
// NewEnv reads only what the process environment says verbatim, and
// validate (which does know -target, via its parameter) resolves
// DBName's default. This mirrors app/api/cmd/api/env.go's split
// between NewEnv (no validation, no external input) and validate
// (checks/resolves, using whatever additional context the caller
// supplies). appRole (below) mirrors the same split for the optional
// role request.
//
// AppDBUser/AppDBPassword deliberately have no default and are never
// required: both left unset is this migrator's pre-ISSUE-016 behavior
// (role provisioning skipped entirely, CREATE DATABASE + goose only),
// preserving backward compatibility for any deployment that has not
// yet wired app/iac's api_app/auth_app secrets through.
type Env struct {
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBSSLMode         string
	DBName            string
	DBMaintenanceName string
	MigratorTimeout   time.Duration
	AppDBUser         string
	AppDBPassword     string
}

// NewEnv reads every environment variable app/migrator consumes and
// applies the defaults that do not depend on -target. It performs no
// required-field validation and no -target-dependent defaulting; call
// Env.validate for the DB_* connection settings and Env.appRole for
// the optional role request.
func NewEnv() Env {
	return Env{
		DBHost:            os.Getenv("DB_HOST"),
		DBPort:            orDefault(os.Getenv("DB_PORT"), defaultDBPort),
		DBUser:            os.Getenv("DB_USER"),
		DBPassword:        os.Getenv("DB_PASSWORD"),
		DBSSLMode:         orDefault(os.Getenv("DB_SSLMODE"), defaultSSLMode),
		DBName:            os.Getenv("DB_NAME"),
		DBMaintenanceName: orDefault(os.Getenv("DB_MAINTENANCE_NAME"), defaultMaintenanceName),
		MigratorTimeout:   migratorTimeout(),
		AppDBUser:         os.Getenv("APP_DB_USER"),
		AppDBPassword:     os.Getenv("APP_DB_PASSWORD"),
	}
}

// migratorTimeout resolves the goose run's timeout budget: the
// MIGRATOR_TIMEOUT environment variable if set and parsable as a Go
// time.Duration (e.g. "10m", "90s"), otherwise
// defaultMigrationTimeout. A set-but-unparsable value is treated the
// same as unset (falls back to the default) but is logged as a
// warning so a typo'd override is visible instead of silently
// behaving as if it were never set.
func migratorTimeout() time.Duration {
	raw := os.Getenv("MIGRATOR_TIMEOUT")
	if raw == "" {
		return defaultMigrationTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		slog.Warn("migrator: invalid MIGRATOR_TIMEOUT, using default", "value", raw, "default", defaultMigrationTimeout, "error", err)
		return defaultMigrationTimeout
	}
	return d
}

// dbConfig projects e's already-captured connection settings into
// postgres.Config. It reads no environment variables itself.
func (e Env) dbConfig() postgres.Config {
	return postgres.Config{
		Host:            e.DBHost,
		Port:            e.DBPort,
		User:            e.DBUser,
		Password:        e.DBPassword,
		SSLMode:         e.DBSSLMode,
		MaintenanceName: e.DBMaintenanceName,
	}
}

// validate checks that e's required fields (DB_HOST/DB_USER/
// DB_PASSWORD) are present, and resolves the target database name --
// e.DBName if set, otherwise target itself (target/target's own name
// "api"/"auth", plan §RF.1.1) -- into a migration.DatabaseName. The
// returned error, like app/{api,auth}/infra/postgres.Config.Validate's,
// never echoes DBPassword's value.
func (e Env) validate(target migration.Target) (postgres.Config, migration.DatabaseName, error) {
	var missing []string
	if e.DBHost == "" {
		missing = append(missing, "DB_HOST")
	}
	if e.DBUser == "" {
		missing = append(missing, "DB_USER")
	}
	if e.DBPassword == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if len(missing) > 0 {
		return postgres.Config{}, migration.DatabaseName{}, fmt.Errorf("config from env: missing required variable(s): %s", strings.Join(missing, ", "))
	}

	rawName := e.DBName
	if rawName == "" {
		rawName = target.String()
	}
	dbName, err := migration.ParseDatabaseName(rawName)
	if err != nil {
		return postgres.Config{}, migration.DatabaseName{}, fmt.Errorf("config from env: %w", err)
	}

	return e.dbConfig(), dbName, nil
}

// appRole resolves the optional least-privilege runtime role this
// migrator should provision after a successful "up" migration
// (ISSUE-016 R-c), from APP_DB_USER/APP_DB_PASSWORD -- the *role's
// own* username/password (app/iac's api_app/auth_app secret, plan
// §1.3), not this migrator's own master DB_USER/DB_PASSWORD. Both
// unset (the default) resolves to requested=false: the caller (run, in
// main.go) then skips role provisioning entirely, preserving this
// migrator's pre-ISSUE-016 behavior exactly. Exactly one of the two
// being set is treated as a misconfiguration (most likely a missing
// secret reference in app/iac), not a valid "skip" state, and is
// reported as an error naming whichever variable is missing -- without
// ever echoing AppDBPassword's value, mirroring validate's own
// "never echoes DBPassword" contract.
//
// APP_DB_USER equal to e.DBUser (this migrator's own master user) is
// also rejected (ISSUE-016 review Major-2), rather than silently
// falling through to ensureRole (infra/postgres/role.go): ensureRole
// unconditionally issues "ALTER ROLE <name> WITH PASSWORD ..." against
// whatever role name it is given, with no check that the role is not
// the very one this migrator is currently connected as. If APP_DB_USER
// happened to match the master DB_USER (e.g. an app/iac secret
// reference mistake, or a scoped-role secret rotation script that
// accidentally targets the master secret), this migrator would rewrite
// the master role's own password to the scoped secret's value on every
// "up" run -- silently locking out every other master-credentialed
// connection (including this migrator's own next invocation) and
// leaving master/scoped credentials inconsistent. Rejecting the match
// here, before ensureRole is ever reached, turns that footgun into a
// loud startup failure instead.
func (e Env) appRole(dbName migration.DatabaseName) (role migration.AppRole, password string, requested bool, err error) {
	if e.AppDBUser == "" && e.AppDBPassword == "" {
		return migration.AppRole{}, "", false, nil
	}
	if e.AppDBUser == "" {
		return migration.AppRole{}, "", false, errors.New("config from env: APP_DB_PASSWORD is set but APP_DB_USER is not (both or neither must be set)")
	}
	if e.AppDBPassword == "" {
		return migration.AppRole{}, "", false, errors.New("config from env: APP_DB_USER is set but APP_DB_PASSWORD is not (both or neither must be set)")
	}
	if e.AppDBUser == e.DBUser {
		return migration.AppRole{}, "", false, errors.New("config from env: APP_DB_USER must not equal DB_USER (the master role this migrator connects as); ensureRole would silently overwrite the master role's password with the scoped role's secret")
	}

	role, err = migration.ParseAppRole(e.AppDBUser, dbName)
	if err != nil {
		return migration.AppRole{}, "", false, fmt.Errorf("config from env: %w", err)
	}
	return role, e.AppDBPassword, true, nil
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
