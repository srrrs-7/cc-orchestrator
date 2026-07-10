package main

import (
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
)

// setAllDBEnv sets every DB_* variable NewEnv reads to an explicit
// value for the duration of the calling test (t.Setenv auto-restores
// on cleanup), so each subtest starts from a fully known environment
// instead of inheriting whatever the process happened to have set
// (SPEC-005 plan §RF.6.2 RB2: table-driven, no reliance on execution
// order). Setting a variable to "" is equivalent to leaving it unset
// for every consumer in this package (they all check os.Getenv(...)
// == ""). Moved from the pre-refactor config_test.go's setAllDBEnv.
func setAllDBEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD",
		"DB_SSLMODE", "DB_NAME", "DB_MAINTENANCE_NAME",
	} {
		t.Setenv(key, env[key])
	}
}

func mustParseTarget(t *testing.T, s string) migration.Target {
	t.Helper()
	target, err := migration.ParseTarget(s)
	if err != nil {
		t.Fatalf("migration.ParseTarget(%q) unexpected error: %v", s, err)
	}
	return target
}

// TestEnv_Validate_DefaultDatabaseNamePerTarget is the R5/§RF.1.1
// "DB_NAME defaults to target itself" contract: with DB_NAME unset,
// api gets database "api" and auth gets database "auth" -- the two
// stacks are separated by database, not by a shared schema/
// search_path selector. Moved from the pre-refactor config_test.go's
// TestConfigFromEnv_DefaultDatabaseNamePerTarget.
func TestEnv_Validate_DefaultDatabaseNamePerTarget(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		wantDBName string
	}{
		{name: "target api -> DB_NAME defaults to api", target: "api", wantDBName: "api"},
		{name: "target auth -> DB_NAME defaults to auth", target: "auth", wantDBName: "auth"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setAllDBEnv(t, map[string]string{
				"DB_HOST":     "db.internal",
				"DB_USER":     "app",
				"DB_PASSWORD": "pw",
				// DB_NAME deliberately left unset (empty).
			})

			e := NewEnv()
			_, dbName, err := e.validate(mustParseTarget(t, tc.target))
			if err != nil {
				t.Fatalf("Env.validate(%q) unexpected error: %v", tc.target, err)
			}
			if dbName.String() != tc.wantDBName {
				t.Errorf("Env.validate(%q) database name = %q, want %q", tc.target, dbName, tc.wantDBName)
			}
		})
	}
}

// TestEnv_Validate_DBNameExplicitOverride asserts an explicit DB_NAME
// wins over the target-derived default. Moved from the pre-refactor
// config_test.go's TestConfigFromEnv_DBNameExplicitOverride.
func TestEnv_Validate_DBNameExplicitOverride(t *testing.T) {
	setAllDBEnv(t, map[string]string{
		"DB_HOST":     "db.internal",
		"DB_USER":     "app",
		"DB_PASSWORD": "pw",
		"DB_NAME":     "custom_db",
	})

	e := NewEnv()
	_, dbName, err := e.validate(mustParseTarget(t, "api"))
	if err != nil {
		t.Fatalf("Env.validate() unexpected error: %v", err)
	}
	if dbName.String() != "custom_db" {
		t.Errorf("Env.validate() database name = %q, want %q (explicit DB_NAME must win over the api default)", dbName, "custom_db")
	}
}

// TestEnv_Defaults covers the remaining defaulted fields: DB_PORT,
// DB_SSLMODE and DB_MAINTENANCE_NAME. DB_SSLMODE's default is
// fail-closed ("require", ISSUE-016 m-2): omitting it must not
// silently downgrade this migrator's (master-credentialed) connection
// to plaintext. Moved from the pre-refactor config_test.go's
// TestConfigFromEnv_Defaults.
func TestEnv_Defaults(t *testing.T) {
	setAllDBEnv(t, map[string]string{
		"DB_HOST":     "db.internal",
		"DB_USER":     "app",
		"DB_PASSWORD": "pw",
	})

	e := NewEnv()
	if e.DBPort != "5432" {
		t.Errorf("NewEnv().DBPort = %q, want default %q", e.DBPort, "5432")
	}
	if e.DBSSLMode != "require" {
		t.Errorf("NewEnv().DBSSLMode = %q, want default %q", e.DBSSLMode, "require")
	}
	if e.DBMaintenanceName != "postgres" {
		t.Errorf("NewEnv().DBMaintenanceName = %q, want default %q", e.DBMaintenanceName, "postgres")
	}
}

// TestEnv_MaintenanceNameExplicitOverride covers DB_MAINTENANCE_NAME's
// escape hatch (some RDS deployments may not expose a usable
// "postgres" database). Moved from the pre-refactor config_test.go's
// TestConfigFromEnv_MaintenanceNameExplicitOverride.
func TestEnv_MaintenanceNameExplicitOverride(t *testing.T) {
	setAllDBEnv(t, map[string]string{
		"DB_HOST":             "db.internal",
		"DB_USER":             "app",
		"DB_PASSWORD":         "pw",
		"DB_MAINTENANCE_NAME": "bootstrap",
	})

	e := NewEnv()
	if e.DBMaintenanceName != "bootstrap" {
		t.Errorf("NewEnv().DBMaintenanceName = %q, want %q", e.DBMaintenanceName, "bootstrap")
	}
}

// TestEnv_Validate_MissingRequired is the fail-closed contract for the
// three variables with no default (DB_HOST/DB_USER/DB_PASSWORD,
// mirroring app/{api,auth}/infra/postgres.Config.Validate): a missing
// required variable is reported by name, and the error never echoes
// DB_PASSWORD's value even when it was one of the fields present.
// Moved from the pre-refactor config_test.go's
// TestConfigFromEnv_MissingRequired.
func TestEnv_Validate_MissingRequired(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		want    []string // variable names the error must mention
		mustNot []string // variable names the error must NOT mention (were set)
	}{
		{
			name:    "missing DB_HOST only",
			env:     map[string]string{"DB_USER": "app", "DB_PASSWORD": "pw"},
			want:    []string{"DB_HOST"},
			mustNot: []string{"DB_USER", "DB_PASSWORD"},
		},
		{
			name:    "missing DB_USER only",
			env:     map[string]string{"DB_HOST": "db.internal", "DB_PASSWORD": "pw"},
			want:    []string{"DB_USER"},
			mustNot: []string{"DB_HOST", "DB_PASSWORD"},
		},
		{
			name:    "missing DB_PASSWORD only",
			env:     map[string]string{"DB_HOST": "db.internal", "DB_USER": "app"},
			want:    []string{"DB_PASSWORD"},
			mustNot: []string{"DB_HOST", "DB_USER"},
		},
		{
			name:    "missing all three",
			env:     map[string]string{},
			want:    []string{"DB_HOST", "DB_USER", "DB_PASSWORD"},
			mustNot: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setAllDBEnv(t, tc.env)

			e := NewEnv()
			_, _, err := e.validate(mustParseTarget(t, "api"))
			if err == nil {
				t.Fatalf("Env.validate() = nil error, want an error naming %v", tc.want)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Env.validate() error = %q, want it to mention missing variable %s", err.Error(), want)
				}
			}
			for _, notWant := range tc.mustNot {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("Env.validate() error = %q, unexpectedly names %s, which was set", err.Error(), notWant)
				}
			}
		})
	}
}

// TestEnv_Validate_MissingRequiredNeverLeaksPassword is the R6
// security assertion: when DB_PASSWORD is set to a secret value but
// another required variable is missing, the resulting error must
// never contain that secret value. Moved from the pre-refactor
// config_test.go's TestConfigFromEnv_MissingRequiredNeverLeaksPassword.
func TestEnv_Validate_MissingRequiredNeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-s3cret-migrator-password"
	setAllDBEnv(t, map[string]string{
		"DB_USER":     "app",
		"DB_PASSWORD": secret,
		// DB_HOST deliberately left unset so Env.validate errors.
	})

	e := NewEnv()
	_, _, err := e.validate(mustParseTarget(t, "api"))
	if err == nil {
		t.Fatal("Env.validate() = nil error, want an error (DB_HOST missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Env.validate() error %q leaks the DB_PASSWORD value", err.Error())
	}
}

// setAppRoleEnv sets APP_DB_USER/APP_DB_PASSWORD explicitly for the
// duration of the calling test (t.Setenv auto-restores on cleanup), so
// Env.appRole tests are hermetic regardless of whatever the ambient
// process environment happens to have set -- mirrors setAllDBEnv's
// rationale for the DB_* variables (SPEC-005 plan §RF.6.2 RB2).
//
// It also pins DB_USER (e.DBUser, the master user Env.appRole compares
// APP_DB_USER against, ISSUE-016 review Major-2) to a fixed value
// distinct from every APP_DB_USER literal used elsewhere in this file
// ("api_app", "API_APP; drop role x", ""), so those existing tests
// never accidentally collide with the master-match check exercised by
// TestEnv_AppRole_UserEqualsMaster_Errors below. Tests that specifically
// need DB_USER to equal APP_DB_USER set it themselves afterward via
// their own t.Setenv("DB_USER", ...) call (which wins, since it runs
// later in the same test).
func setAppRoleEnv(t *testing.T, appDBUser, appDBPassword string) {
	t.Helper()
	t.Setenv("DB_USER", "migrator_master_user")
	t.Setenv("APP_DB_USER", appDBUser)
	t.Setenv("APP_DB_PASSWORD", appDBPassword)
}

// mustDatabaseName parses a literal known-valid database name for
// testing Env.appRole, which takes an already-resolved
// migration.DatabaseName (Env.validate's job, covered by the tests
// above) rather than resolving one itself from -target/DB_NAME.
func mustDatabaseName(t *testing.T, name string) migration.DatabaseName {
	t.Helper()
	dbName, err := migration.ParseDatabaseName(name)
	if err != nil {
		t.Fatalf("migration.ParseDatabaseName(%q) unexpected error: %v", name, err)
	}
	return dbName
}

// TestEnv_AppRole_BothUnset_NotRequested is ISSUE-016 R-c's backward
// compatibility contract: a deployment that has not wired app/iac's
// api_app/auth_app secrets through (APP_DB_USER/APP_DB_PASSWORD both
// left unset, this migrator's pre-ISSUE-016 default) must resolve to
// requested=false with no error, so main.go's run skips role
// provisioning entirely and this migrator's behavior is unchanged
// (CREATE DATABASE + goose only).
func TestEnv_AppRole_BothUnset_NotRequested(t *testing.T) {
	setAppRoleEnv(t, "", "")

	e := NewEnv()
	role, password, requested, err := e.appRole(mustDatabaseName(t, "api"))
	if err != nil {
		t.Fatalf("Env.appRole() unexpected error: %v", err)
	}
	if requested {
		t.Errorf("Env.appRole() requested = true, want false (both APP_DB_USER/APP_DB_PASSWORD unset)")
	}
	if password != "" {
		t.Errorf("Env.appRole() password = %q, want empty when not requested", password)
	}
	if role != (migration.AppRole{}) {
		t.Errorf("Env.appRole() role = %+v, want the zero value when not requested", role)
	}
}

// TestEnv_AppRole_ExactlyOneSet_Errors is the "misconfiguration, not a
// valid skip state" contract (env.go's appRole doc comment): exactly
// one of APP_DB_USER/APP_DB_PASSWORD being set most likely means an
// app/iac secret reference is missing, and must fail loudly rather
// than silently behave as if role provisioning were never requested.
func TestEnv_AppRole_ExactlyOneSet_Errors(t *testing.T) {
	cases := []struct {
		name          string
		appDBUser     string
		appDBPassword string
		wantMentions  string // variable name the error must name (the one left unset)
	}{
		{
			name:          "only APP_DB_USER set",
			appDBUser:     "api_app",
			appDBPassword: "",
			wantMentions:  "APP_DB_PASSWORD",
		},
		{
			name:          "only APP_DB_PASSWORD set",
			appDBUser:     "",
			appDBPassword: "pw",
			wantMentions:  "APP_DB_USER",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setAppRoleEnv(t, tc.appDBUser, tc.appDBPassword)

			e := NewEnv()
			_, _, requested, err := e.appRole(mustDatabaseName(t, "api"))
			if err == nil {
				t.Fatalf("Env.appRole() = nil error, want an error naming %s", tc.wantMentions)
			}
			if requested {
				t.Errorf("Env.appRole() requested = true, want false when misconfigured")
			}
			if !strings.Contains(err.Error(), tc.wantMentions) {
				t.Errorf("Env.appRole() error = %q, want it to mention %s", err.Error(), tc.wantMentions)
			}
		})
	}
}

// TestEnv_AppRole_ExactlyOneSet_NeverLeaksPassword mirrors
// TestEnv_Validate_MissingRequiredNeverLeaksPassword for the
// APP_DB_PASSWORD-only-set case: the resulting misconfiguration error
// must never echo the secret value.
func TestEnv_AppRole_ExactlyOneSet_NeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-s3cret-app-role-password"
	setAppRoleEnv(t, "", secret)

	e := NewEnv()
	_, _, _, err := e.appRole(mustDatabaseName(t, "api"))
	if err == nil {
		t.Fatal("Env.appRole() = nil error, want an error (APP_DB_USER missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Env.appRole() error %q leaks the APP_DB_PASSWORD value", err.Error())
	}
}

// TestEnv_AppRole_BothSet_ResolvesRole is ISSUE-016 R-c's main
// resolution path: both variables set names the least-privilege
// runtime role (paired with the caller-supplied DatabaseName, not
// re-derived) and carries its password through unmodified.
func TestEnv_AppRole_BothSet_ResolvesRole(t *testing.T) {
	setAppRoleEnv(t, "api_app", "pw")

	e := NewEnv()
	dbName := mustDatabaseName(t, "api")
	role, password, requested, err := e.appRole(dbName)
	if err != nil {
		t.Fatalf("Env.appRole() unexpected error: %v", err)
	}
	if !requested {
		t.Fatal("Env.appRole() requested = false, want true (both APP_DB_USER/APP_DB_PASSWORD set)")
	}
	if role.Name() != "api_app" {
		t.Errorf("Env.appRole() role.Name() = %q, want %q", role.Name(), "api_app")
	}
	if role.Database().String() != dbName.String() {
		t.Errorf("Env.appRole() role.Database() = %q, want %q (the dbName argument, unmodified)", role.Database(), dbName)
	}
	if password != "pw" {
		t.Errorf("Env.appRole() password = %q, want %q", password, "pw")
	}
}

// TestEnv_AppRole_UserEqualsMaster_Errors is ISSUE-016 review Major-2's
// regression guard: APP_DB_USER set to the exact same value as this
// migrator's own master DB_USER must be rejected as a misconfiguration
// (env.go's appRole doc comment explains why -- ensureRole would
// otherwise silently ALTER ROLE the master user's own password to the
// scoped secret's value). Also asserts the resulting error never
// echoes APP_DB_PASSWORD's value, mirroring this file's other
// never-leaks-password tests.
func TestEnv_AppRole_UserEqualsMaster_Errors(t *testing.T) {
	const secret = "sup3r-s3cret-would-be-master-overwrite"
	setAppRoleEnv(t, "shared_user", secret)
	t.Setenv("DB_USER", "shared_user") // overrides setAppRoleEnv's default, deliberately matching APP_DB_USER

	e := NewEnv()
	_, _, requested, err := e.appRole(mustDatabaseName(t, "api"))
	if err == nil {
		t.Fatal("Env.appRole() = nil error, want an error (APP_DB_USER equals master DB_USER)")
	}
	if requested {
		t.Errorf("Env.appRole() requested = true, want false when APP_DB_USER equals master DB_USER")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Env.appRole() error %q leaks the APP_DB_PASSWORD value", err.Error())
	}
}

// TestEnv_AppRole_InvalidIdentifier_Errors asserts a malformed
// APP_DB_USER (violating migration.ParseAppRole's safe-identifier
// allowlist, e.g. an app/iac misconfiguration or injection-shaped
// value) is rejected rather than silently accepted -- appRole must not
// bypass ParseAppRole's validation.
func TestEnv_AppRole_InvalidIdentifier_Errors(t *testing.T) {
	setAppRoleEnv(t, "API_APP; drop role x", "pw")

	e := NewEnv()
	_, _, requested, err := e.appRole(mustDatabaseName(t, "api"))
	if err == nil {
		t.Fatal("Env.appRole() = nil error, want an error (APP_DB_USER is not a safe identifier)")
	}
	if requested {
		t.Errorf("Env.appRole() requested = true, want false when APP_DB_USER is invalid")
	}
}
