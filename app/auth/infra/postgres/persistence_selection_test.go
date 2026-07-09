// persistence_selection_test.go exercises the "DB_HOST" / "APP_ENV"
// persistence-selection contract from docs/plans/SPEC-005-plan.md §0
// "切替の env / DSN / 本番必須強制" (SPEC-005 R6), as it applies to
// app/auth's cmd/authz wiring (client / user / authcode repositories,
// plus which seed path runs):
//
//	DB_HOST set                     -> Postgres, regardless of APP_ENV
//	DB_HOST unset, APP_ENV=local    -> memory
//	DB_HOST unset, APP_ENV=test     -> memory
//	DB_HOST unset, otherwise        -> fail-closed error (no memory
//	                                    fallback; this includes
//	                                    APP_ENV=production, an unset
//	                                    APP_ENV, and any unrecognized
//	                                    APP_ENV value)
//
// This file carries no build tag: selecting a persistence mode and
// assembling a DSN are pure, DB-independent decisions (plan §5.1 "選択
// ロジック/DSN 組み立て(ユニット・DB 非依存)"), so it runs as part of the
// default `make test` and requires no live Postgres.
//
// SPEC-005 phase E1 note: this file replaces the phase B1 outline
// (persistence_selection_outline_test.go, all-t.Skip) now that
// impl-db has landed the real postgres.SelectMode /
// postgres.ConfigFromEnv / (postgres.Config).DSN() in db.go. The
// behavioral target table below is unchanged from the outline.
//
// Unlike app/api's ConfigFromEnv (which takes an injected
// func(string) string), app/auth's ConfigFromEnv reads os.Getenv
// directly and returns (Config, error) -- so the ConfigFromEnv-level
// tests below use t.Setenv (auto-restored per test) rather than a
// map-backed getenv, and unset a variable by setting it to "" (which
// os.Getenv-based ConfigFromEnv treats identically to "not present",
// since it only ever checks for the empty string).
package postgres_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
)

func TestSelectMode(t *testing.T) {
	cases := []struct {
		name     string
		dbHost   string
		appEnv   string
		wantMode postgres.Mode
		wantErr  bool // fail-closed: no repositories, an error instead
	}{
		{
			name:     "DB_HOST set, APP_ENV=production -> postgres",
			dbHost:   "db.internal",
			appEnv:   "production",
			wantMode: postgres.ModePostgres,
		},
		{
			name:     "DB_HOST set, APP_ENV unset -> postgres (DB_HOST alone is sufficient)",
			dbHost:   "db.internal",
			wantMode: postgres.ModePostgres,
		},
		{
			name:     "DB_HOST unset, APP_ENV=local -> memory",
			appEnv:   "local",
			wantMode: postgres.ModeMemory,
		},
		{
			name:     "DB_HOST unset, APP_ENV=test -> memory",
			appEnv:   "test",
			wantMode: postgres.ModeMemory,
		},
		{
			name:    "DB_HOST unset, APP_ENV=production -> fail-closed error, no memory fallback",
			appEnv:  "production",
			wantErr: true,
		},
		{
			name:    "DB_HOST unset, APP_ENV unset entirely (the real default) -> fail-closed error",
			wantErr: true,
		},
		{
			name:    "DB_HOST unset, APP_ENV set to an unrecognized value -> fail-closed error, not silently memory",
			appEnv:  "staging",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := postgres.SelectMode(tc.dbHost, tc.appEnv)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SelectMode(%q, %q) = (%q, nil), want a fail-closed error", tc.dbHost, tc.appEnv, mode)
				}
				if !errors.Is(err, postgres.ErrPersistenceNotConfigured) {
					t.Errorf("SelectMode(%q, %q) error = %v, want wrapping %v", tc.dbHost, tc.appEnv, err, postgres.ErrPersistenceNotConfigured)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectMode(%q, %q) unexpected error: %v", tc.dbHost, tc.appEnv, err)
			}
			if mode != tc.wantMode {
				t.Errorf("SelectMode(%q, %q) = %q, want %q", tc.dbHost, tc.appEnv, mode, tc.wantMode)
			}
		})
	}
}

// TestSelectMode_FailClosedErrorNeverLeaksDBPassword covers the R6
// security requirement that a fail-closed configuration error is safe
// to log: SelectMode's signature does not even accept a password, so
// the error it returns cannot echo one.
func TestSelectMode_FailClosedErrorNeverLeaksDBPassword(t *testing.T) {
	_, err := postgres.SelectMode("", "production")
	if err == nil {
		t.Fatal("SelectMode(\"\", \"production\") = nil error, want a fail-closed error")
	}
	const secret = "sup3r-s3cret-db-password"
	if strings.Contains(err.Error(), secret) {
		t.Errorf("SelectMode() error %q unexpectedly contains a password-shaped value", err.Error())
	}
}

// setDBEnv sets every discrete DB_* variable ConfigFromEnv reads,
// using t.Setenv (which auto-restores the prior value once the test
// ends) so this test never leaks environment mutations into other
// tests in this package -- including the //go:build integration
// tests, which read the same DB_* variables to reach a real database.
// An empty string for a given field simulates that variable being
// unset, since ConfigFromEnv only ever checks os.Getenv(...) == "".
func setDBEnv(t *testing.T, host, port, name, user, password, sslmode, schema string) {
	t.Helper()
	t.Setenv("DB_HOST", host)
	t.Setenv("DB_PORT", port)
	t.Setenv("DB_NAME", name)
	t.Setenv("DB_USER", user)
	t.Setenv("DB_PASSWORD", password)
	t.Setenv("DB_SSLMODE", sslmode)
	t.Setenv("DB_SCHEMA", schema)
}

// TestConfigFromEnv_ReflectsEveryDiscreteVar asserts every DB_* knob
// ConfigFromEnv reads is represented both in the returned Config and
// in the DSN string (postgres.Config).DSN() assembles from it
// (order-independent field checks, not a fixed string layout), and
// that api/auth resolve to different schemas from the same
// DB_HOST/DB_NAME (only DB_SCHEMA differs between the stacks).
func TestConfigFromEnv_ReflectsEveryDiscreteVar(t *testing.T) {
	setDBEnv(t, "db.internal", "6543", "appdb", "appuser", "s3cret-pw", "require", "auth")

	cfg, err := postgres.ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() unexpected error: %v", err)
	}
	if cfg.Host != "db.internal" {
		t.Errorf("Config.Host = %q, want %q", cfg.Host, "db.internal")
	}
	if cfg.Port != "6543" {
		t.Errorf("Config.Port = %q, want %q", cfg.Port, "6543")
	}
	if cfg.Name != "appdb" {
		t.Errorf("Config.Name = %q, want %q", cfg.Name, "appdb")
	}
	if cfg.User != "appuser" {
		t.Errorf("Config.User = %q, want %q", cfg.User, "appuser")
	}
	if cfg.Password != "s3cret-pw" {
		t.Errorf("Config.Password = %q, want %q", cfg.Password, "s3cret-pw")
	}
	if cfg.SSLMode != "require" {
		t.Errorf("Config.SSLMode = %q, want %q", cfg.SSLMode, "require")
	}
	if cfg.Schema != "auth" {
		t.Errorf("Config.Schema = %q, want %q (app/auth's schema, distinct from app/api's \"api\")", cfg.Schema, "auth")
	}

	dsn := cfg.DSN()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(DSN()) unexpected error: %v", err)
	}
	if u.Scheme != "postgres" {
		t.Errorf("DSN() scheme = %q, want %q", u.Scheme, "postgres")
	}
	if u.Hostname() != "db.internal" {
		t.Errorf("DSN() host = %q, want %q", u.Hostname(), "db.internal")
	}
	if u.Port() != "6543" {
		t.Errorf("DSN() port = %q, want %q", u.Port(), "6543")
	}
	if got, want := strings.TrimPrefix(u.Path, "/"), "appdb"; got != want {
		t.Errorf("DSN() path = %q, want %q", got, want)
	}
	if u.User.Username() != "appuser" {
		t.Errorf("DSN() user = %q, want %q", u.User.Username(), "appuser")
	}
	if pw, _ := u.User.Password(); pw != "s3cret-pw" {
		t.Errorf("DSN() password = %q, want %q", pw, "s3cret-pw")
	}
	if got := u.Query().Get("sslmode"); got != "require" {
		t.Errorf("DSN() sslmode = %q, want %q", got, "require")
	}
	if got := u.Query().Get("search_path"); got != "auth" {
		t.Errorf("DSN() search_path = %q, want %q (R3: schema separation via search_path)", got, "auth")
	}
}

// TestConfigFromEnv_Defaults confirms DB_PORT/DB_SSLMODE/DB_SCHEMA
// fall back to safe local-development defaults when unset, while the
// four required settings carry no default. app/auth's default schema
// ("auth") is asserted to differ from app/api's ("api"), matching
// docs/plans/SPEC-005-plan.md §0 "スキーマ分離機構".
func TestConfigFromEnv_Defaults(t *testing.T) {
	setDBEnv(t, "db.internal", "", "appdb", "appuser", "pw", "", "")

	cfg, err := postgres.ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() unexpected error: %v", err)
	}
	if cfg.Port != "5432" {
		t.Errorf("Config.Port default = %q, want %q", cfg.Port, "5432")
	}
	if cfg.SSLMode != "disable" {
		t.Errorf("Config.SSLMode default = %q, want %q", cfg.SSLMode, "disable")
	}
	if cfg.Schema != "auth" {
		t.Errorf("Config.Schema default = %q, want %q (app/auth's schema)", cfg.Schema, "auth")
	}
}

// TestConfigFromEnv_MissingRequiredVar_RejectsExplicitly asserts that
// ConfigFromEnv rejects a missing required variable (e.g. DB_NAME,
// with DB_HOST otherwise set) with an explicit error naming it,
// rather than returning a Config whose empty Name would later be
// silently embedded in a malformed DSN passed to sql.Open.
func TestConfigFromEnv_MissingRequiredVar_RejectsExplicitly(t *testing.T) {
	setDBEnv(t, "db.internal", "5432", "" /* DB_NAME missing */, "appuser", "pw", "disable", "auth")

	cfg, err := postgres.ConfigFromEnv()
	if err == nil {
		t.Fatalf("ConfigFromEnv() = (%+v, nil), want an error naming the missing DB_NAME", cfg)
	}
	if !strings.Contains(err.Error(), "DB_NAME") {
		t.Errorf("ConfigFromEnv() error = %q, want it to name the missing variable DB_NAME", err.Error())
	}
}

// TestConfigFromEnv_ErrorNeverLeaksPassword is the R6 security
// assertion for ConfigFromEnv: when validation fails, the returned
// error lists only missing variable *names* -- never DB_PASSWORD's
// actual value, even though DB_PASSWORD was supplied alongside a
// missing required field.
func TestConfigFromEnv_ErrorNeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-s3cret-db-password"
	// DB_HOST/DB_NAME/DB_USER all deliberately empty; only DB_PASSWORD is set.
	setDBEnv(t, "", "5432", "", "", secret, "disable", "auth")

	_, err := postgres.ConfigFromEnv()
	if err == nil {
		t.Fatal("ConfigFromEnv() = nil error, want an error (DB_HOST/DB_NAME/DB_USER all missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("ConfigFromEnv() error %q leaks the DB_PASSWORD value", err.Error())
	}
	for _, want := range []string{"DB_HOST", "DB_NAME", "DB_USER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ConfigFromEnv() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
}
