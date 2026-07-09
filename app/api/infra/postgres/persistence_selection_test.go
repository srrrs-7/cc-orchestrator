// persistence_selection_test.go exercises the "DB_HOST" / "APP_ENV"
// persistence-selection contract from docs/plans/SPEC-005-plan.md §0
// "切替の env / DSN / 本番必須強制" (SPEC-005 R6):
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
package postgres_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
)

// mapGetenv adapts a plain map[string]string into the
// func(string) string shape postgres.SelectMode/ConfigFromEnv expect,
// letting these tests exercise those pure functions directly without
// mutating any real process environment variable (os.Setenv/
// t.Setenv), matching plan §5.1's "DB-independent" premise.
func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func TestSelectMode(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		wantMode postgres.Mode
		wantErr  bool // fail-closed: no Repository, an error instead
	}{
		{
			name:     "DB_HOST set, APP_ENV=production -> postgres",
			env:      map[string]string{"DB_HOST": "db.internal", "APP_ENV": "production"},
			wantMode: postgres.ModePostgres,
		},
		{
			name:     "DB_HOST set, APP_ENV unset -> postgres (DB_HOST alone is sufficient)",
			env:      map[string]string{"DB_HOST": "db.internal"},
			wantMode: postgres.ModePostgres,
		},
		{
			name:     "DB_HOST unset, APP_ENV=local -> memory",
			env:      map[string]string{"APP_ENV": "local"},
			wantMode: postgres.ModeMemory,
		},
		{
			name:     "DB_HOST unset, APP_ENV=test -> memory",
			env:      map[string]string{"APP_ENV": "test"},
			wantMode: postgres.ModeMemory,
		},
		{
			name:    "DB_HOST unset, APP_ENV=production -> fail-closed error, no memory fallback",
			env:     map[string]string{"APP_ENV": "production"},
			wantErr: true,
		},
		{
			name:    "DB_HOST unset, APP_ENV unset entirely (the real default) -> fail-closed error",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name:    "DB_HOST unset, APP_ENV set to an unrecognized value -> fail-closed error, not silently memory",
			env:     map[string]string{"APP_ENV": "staging"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := postgres.SelectMode(mapGetenv(tc.env))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SelectMode() = (%q, nil), want a fail-closed error", mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectMode() unexpected error: %v", err)
			}
			if mode != tc.wantMode {
				t.Errorf("SelectMode() = %q, want %q", mode, tc.wantMode)
			}
		})
	}
}

// TestSelectMode_FailClosedErrorNeverLeaksDBPassword covers the R6
// security requirement that a fail-closed configuration error is safe
// to log: SelectMode itself never even receives DB_PASSWORD, so the
// error it returns cannot echo it regardless of the value.
func TestSelectMode_FailClosedErrorNeverLeaksDBPassword(t *testing.T) {
	const secret = "sup3r-s3cret-db-password"
	env := map[string]string{
		"APP_ENV":     "production",
		"DB_PASSWORD": secret, // present in the environment, but irrelevant to SelectMode's decision
	}

	_, err := postgres.SelectMode(mapGetenv(env))
	if err == nil {
		t.Fatal("SelectMode() = nil error, want a fail-closed error (APP_ENV=production, DB_HOST unset)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("SelectMode() error %q leaks DB_PASSWORD value", err.Error())
	}
}

// TestConfigFromEnv_ReflectsEveryDiscreteVar asserts every DB_* knob
// ConfigFromEnv reads is represented both in the returned Config and
// in the DSN string (postgres.Config).DSN() assembles from it
// (order-independent field checks, not a fixed string layout).
func TestConfigFromEnv_ReflectsEveryDiscreteVar(t *testing.T) {
	env := map[string]string{
		"DB_HOST":     "db.internal",
		"DB_PORT":     "6543",
		"DB_NAME":     "appdb",
		"DB_USER":     "appuser",
		"DB_PASSWORD": "s3cret-pw",
		"DB_SSLMODE":  "require",
		"DB_SCHEMA":   "api",
	}

	cfg := postgres.ConfigFromEnv(mapGetenv(env))
	if cfg.Host != env["DB_HOST"] {
		t.Errorf("Config.Host = %q, want %q", cfg.Host, env["DB_HOST"])
	}
	if cfg.Port != env["DB_PORT"] {
		t.Errorf("Config.Port = %q, want %q", cfg.Port, env["DB_PORT"])
	}
	if cfg.Name != env["DB_NAME"] {
		t.Errorf("Config.Name = %q, want %q", cfg.Name, env["DB_NAME"])
	}
	if cfg.User != env["DB_USER"] {
		t.Errorf("Config.User = %q, want %q", cfg.User, env["DB_USER"])
	}
	if cfg.Password != env["DB_PASSWORD"] {
		t.Errorf("Config.Password = %q, want %q", cfg.Password, env["DB_PASSWORD"])
	}
	if cfg.SSLMode != env["DB_SSLMODE"] {
		t.Errorf("Config.SSLMode = %q, want %q", cfg.SSLMode, env["DB_SSLMODE"])
	}
	if cfg.Schema != env["DB_SCHEMA"] {
		t.Errorf("Config.Schema = %q, want %q", cfg.Schema, env["DB_SCHEMA"])
	}

	dsn, err := cfg.DSN()
	if err != nil {
		t.Fatalf("DSN() unexpected error: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(DSN()) unexpected error: %v", err)
	}
	if u.Scheme != "postgres" {
		t.Errorf("DSN() scheme = %q, want %q", u.Scheme, "postgres")
	}
	if u.Hostname() != env["DB_HOST"] {
		t.Errorf("DSN() host = %q, want %q", u.Hostname(), env["DB_HOST"])
	}
	if u.Port() != env["DB_PORT"] {
		t.Errorf("DSN() port = %q, want %q", u.Port(), env["DB_PORT"])
	}
	if got, want := strings.TrimPrefix(u.Path, "/"), env["DB_NAME"]; got != want {
		t.Errorf("DSN() path = %q, want %q", got, want)
	}
	if u.User.Username() != env["DB_USER"] {
		t.Errorf("DSN() user = %q, want %q", u.User.Username(), env["DB_USER"])
	}
	if pw, _ := u.User.Password(); pw != env["DB_PASSWORD"] {
		t.Errorf("DSN() password = %q, want %q", pw, env["DB_PASSWORD"])
	}
	if got := u.Query().Get("sslmode"); got != env["DB_SSLMODE"] {
		t.Errorf("DSN() sslmode = %q, want %q", got, env["DB_SSLMODE"])
	}
	if got := u.Query().Get("search_path"); got != env["DB_SCHEMA"] {
		t.Errorf("DSN() search_path = %q, want %q (R3: schema separation via search_path)", got, env["DB_SCHEMA"])
	}
}

// TestConfigFromEnv_Defaults confirms DB_PORT/DB_SSLMODE/DB_SCHEMA
// fall back to safe local-development defaults when unset, while the
// four required settings carry no default.
func TestConfigFromEnv_Defaults(t *testing.T) {
	env := map[string]string{
		"DB_HOST":     "db.internal",
		"DB_NAME":     "appdb",
		"DB_USER":     "appuser",
		"DB_PASSWORD": "pw",
		// DB_PORT / DB_SSLMODE / DB_SCHEMA deliberately absent.
	}

	cfg := postgres.ConfigFromEnv(mapGetenv(env))
	if cfg.Port != "5432" {
		t.Errorf("Config.Port default = %q, want %q", cfg.Port, "5432")
	}
	if cfg.SSLMode != "disable" {
		t.Errorf("Config.SSLMode default = %q, want %q", cfg.SSLMode, "disable")
	}
	if cfg.Schema != "api" {
		t.Errorf("Config.Schema default = %q, want %q (app/api's schema)", cfg.Schema, "api")
	}
}

// TestConfigDSN_MissingRequiredVar_RejectsExplicitly asserts that a
// Config missing a required field (e.g. DB_NAME, with DB_HOST
// otherwise set) is rejected by DSN() with an explicit error naming
// the missing variable, rather than silently producing a malformed
// connection string that would later be passed to sql.Open.
func TestConfigDSN_MissingRequiredVar_RejectsExplicitly(t *testing.T) {
	env := map[string]string{
		"DB_HOST":     "db.internal",
		"DB_USER":     "appuser",
		"DB_PASSWORD": "pw",
		// DB_NAME deliberately absent.
	}

	cfg := postgres.ConfigFromEnv(mapGetenv(env))
	dsn, err := cfg.DSN()
	if err == nil {
		t.Fatalf("DSN() = (%q, nil), want an error naming the missing DB_NAME", dsn)
	}
	if !strings.Contains(err.Error(), "DB_NAME") {
		t.Errorf("DSN() error = %q, want it to name the missing variable DB_NAME", err.Error())
	}
}

// TestConfigDSN_ErrorNeverLeaksPassword is the R6 security assertion
// for DSN(): when validation fails, the returned error lists only
// missing variable *names* -- never DB_PASSWORD's actual value, even
// though Password was supplied alongside the missing required field.
func TestConfigDSN_ErrorNeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-s3cret-db-password"
	cfg := postgres.Config{
		// DB_HOST/DB_NAME/DB_USER all deliberately empty; only Password is set.
		Password: secret,
	}

	_, err := cfg.DSN()
	if err == nil {
		t.Fatal("DSN() = nil error, want an error (Host/Name/User all missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("DSN() error %q leaks the DB_PASSWORD value", err.Error())
	}
	for _, want := range []string{"DB_HOST", "DB_NAME", "DB_USER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("DSN() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
}
