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
