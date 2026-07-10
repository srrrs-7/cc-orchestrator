// env_test.go exercises env.go's Env / NewEnv / (Env).validate /
// (Env).dbConfig -- the sole place app/auth reads the process
// environment (os.Getenv), consolidated here by the env.go refactor.
// All os.Getenv reads happen inside NewEnv, so these tests use
// t.Setenv (auto-restored per test, which also neutralizes any
// ambient value already present in the CI/dev environment) to isolate
// each case, and never require a live Postgres: (Env).validate only
// resolves a postgres.Mode and checks presence of DB_* fields via
// postgres.Config.Validate, it never dials a connection.
package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
)

// envVars lists every environment variable NewEnv reads.
var envVars = []string{
	"PORT", "APP_ENV", "ISSUER",
	"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSLMODE",
}

// TestNewEnv_Defaults confirms PORT/ISSUER/DB_PORT/DB_SSLMODE fall back
// to their documented defaults when every relevant variable is unset,
// while variables without a default (APP_ENV, DB_HOST, DB_NAME,
// DB_USER, DB_PASSWORD) stay empty.
func TestNewEnv_Defaults(t *testing.T) {
	for _, key := range envVars {
		t.Setenv(key, "")
	}

	e := NewEnv()

	if e.Port != "8080" {
		t.Errorf("Env.Port = %q, want %q", e.Port, "8080")
	}
	if e.Issuer != "http://localhost:8080" {
		t.Errorf("Env.Issuer = %q, want %q", e.Issuer, "http://localhost:8080")
	}
	if e.DBPort != "5432" {
		t.Errorf("Env.DBPort = %q, want %q", e.DBPort, "5432")
	}
	if e.DBSSLMode != "disable" {
		t.Errorf("Env.DBSSLMode = %q, want %q", e.DBSSLMode, "disable")
	}
	for _, tc := range []struct {
		name string
		got  string
	}{
		{"AppEnv", e.AppEnv},
		{"DBHost", e.DBHost},
		{"DBName", e.DBName},
		{"DBUser", e.DBUser},
		{"DBPassword", e.DBPassword},
	} {
		if tc.got != "" {
			t.Errorf("Env.%s = %q, want empty (no default)", tc.name, tc.got)
		}
	}
}

// TestNewEnv_ReadsEveryVar sets every variable NewEnv reads to a
// distinct value and asserts each is threaded through to the
// corresponding Env field unchanged. This migrates the env-read
// coverage the pre-refactor infra/postgres.ConfigFromEnv tests used to
// carry for the DB_* subset, now consolidated with the non-DB
// variables (including ISSUER) under NewEnv.
func TestNewEnv_ReadsEveryVar(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("APP_ENV", "production")
	t.Setenv("ISSUER", "https://auth.example.com")
	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_NAME", "appdb")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "s3cret-pw")
	t.Setenv("DB_SSLMODE", "require")

	e := NewEnv()

	cases := []struct {
		field string
		got   string
		want  string
	}{
		{"Port", e.Port, "9090"},
		{"AppEnv", e.AppEnv, "production"},
		{"Issuer", e.Issuer, "https://auth.example.com"},
		{"DBHost", e.DBHost, "db.internal"},
		{"DBPort", e.DBPort, "6543"},
		{"DBName", e.DBName, "appdb"},
		{"DBUser", e.DBUser, "appuser"},
		{"DBPassword", e.DBPassword, "s3cret-pw"},
		{"DBSSLMode", e.DBSSLMode, "require"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("Env.%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
}

// TestValidate_MemoryMode_AllowsEmptyDBFields is the 境界値 case for
// validate: in memory mode (APP_ENV=local, DB_HOST unset) every DB_*
// field being its zero value is legal -- validate must not reject an
// otherwise-valid local/test configuration just because DB_* was never
// set.
func TestValidate_MemoryMode_AllowsEmptyDBFields(t *testing.T) {
	e := Env{AppEnv: "local"}

	mode, err := e.validate()
	if err != nil {
		t.Fatalf("validate() unexpected error: %v", err)
	}
	if mode != postgres.ModeMemory {
		t.Errorf("validate() mode = %q, want %q", mode, postgres.ModeMemory)
	}
}

// TestValidate_PostgresMode_RequiresDBFields is the 異常系 case: once
// DB_HOST selects Postgres mode, the remaining required DB_* fields
// (DB_NAME/DB_USER/DB_PASSWORD) become mandatory, and validate's error
// must name every one that is missing.
func TestValidate_PostgresMode_RequiresDBFields(t *testing.T) {
	e := Env{DBHost: "db.internal"} // DBName/DBUser/DBPassword deliberately empty

	_, err := e.validate()
	if err == nil {
		t.Fatal("validate() = nil error, want an error naming the missing DB_NAME/DB_USER/DB_PASSWORD")
	}
	for _, want := range []string{"DB_NAME", "DB_USER", "DB_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validate() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
}

// TestValidate_PostgresMode_ErrorNeverLeaksPassword is the security
// invariant: even though DBPassword is populated (unlike DBName/DBUser
// alongside it), validate's error must never echo its value -- only
// missing variable *names* may appear.
func TestValidate_PostgresMode_ErrorNeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-secret-xyz"
	e := Env{DBHost: "db.internal", DBPassword: secret} // DBName/DBUser deliberately empty

	_, err := e.validate()
	if err == nil {
		t.Fatal("validate() = nil error, want an error (DB_NAME/DB_USER missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("validate() error %q leaks the DB_PASSWORD value", err.Error())
	}
	for _, want := range []string{"DB_NAME", "DB_USER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validate() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
}

// TestValidate_FailClosed is the 異常系 fail-closed case: with every
// field at its zero value (DB_HOST unset, APP_ENV unset), validate
// must not silently default to memory mode -- it must return an error
// wrapping postgres.ErrPersistenceNotConfigured, so callers can
// distinguish "not configured" from other failures via errors.Is.
func TestValidate_FailClosed(t *testing.T) {
	e := Env{}

	_, err := e.validate()
	if err == nil {
		t.Fatal("validate() = nil error, want a fail-closed error (DB_HOST unset, APP_ENV unset)")
	}
	if !errors.Is(err, postgres.ErrPersistenceNotConfigured) {
		t.Errorf("validate() error = %v, want it to wrap %v", err, postgres.ErrPersistenceNotConfigured)
	}
}

// TestValidate_PostgresMode_AllFieldsPresent is the 正常系 case: a
// fully-populated Postgres Env resolves to ModePostgres with no error.
func TestValidate_PostgresMode_AllFieldsPresent(t *testing.T) {
	e := Env{DBHost: "db.internal", DBName: "appdb", DBUser: "appuser", DBPassword: "pw"}

	mode, err := e.validate()
	if err != nil {
		t.Fatalf("validate() unexpected error: %v", err)
	}
	if mode != postgres.ModePostgres {
		t.Errorf("validate() mode = %q, want %q", mode, postgres.ModePostgres)
	}
}
