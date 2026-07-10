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
// env.go refactor note: postgres.SelectMode now takes the two already-
// read string values (dbHost, appEnv) directly rather than an injected
// getenv function, and postgres.ConfigFromEnv is removed entirely --
// cmd/api's env.go (package main) is now the sole place this module
// reads os.Getenv (see cmd/api/env_test.go for that coverage,
// including the defaults previously asserted here via ConfigFromEnv).
// This file now covers only postgres.SelectMode and the pure
// postgres.Config methods (Validate/DSN), built from explicit literals
// instead of an env map.
package postgres_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
)

func TestSelectMode(t *testing.T) {
	cases := []struct {
		name     string
		dbHost   string
		appEnv   string
		wantMode postgres.Mode
		wantErr  bool // fail-closed: no Repository, an error instead
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

// TestSelectMode_FailClosedErrorMentionsAppEnv is the R6 security /
// diagnosability assertion for the fail-closed path: SelectMode's
// signature no longer accepts a password at all (unlike the old
// getenv-function shape), so a leaked-password regression is no longer
// representable here -- what remains to guard is that the error is
// still actionable, i.e. it names APP_ENV so an operator can tell why
// persistence selection failed.
func TestSelectMode_FailClosedErrorMentionsAppEnv(t *testing.T) {
	cases := []struct {
		name   string
		appEnv string
	}{
		{name: "APP_ENV=production", appEnv: "production"},
		{name: "APP_ENV unset", appEnv: ""},
		{name: "APP_ENV unrecognized", appEnv: "staging"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := postgres.SelectMode("", tc.appEnv)
			if err == nil {
				t.Fatalf("SelectMode(\"\", %q) = nil error, want a fail-closed error", tc.appEnv)
			}
			if !strings.Contains(err.Error(), "APP_ENV") {
				t.Errorf("SelectMode(\"\", %q) error = %q, want it to mention APP_ENV", tc.appEnv, err.Error())
			}
		})
	}
}

// TestConfigDSN_ReflectsEveryField asserts every field of a
// fully-populated postgres.Config is represented in the DSN string
// (postgres.Config).DSN() assembles from it (order-independent field
// checks via net/url, not a fixed string layout).
//
// Config carries no Schema field (removed by the 2026-07-09 refactor,
// SPEC-005 plan §RF.2.2): api now connects to its own dedicated
// Postgres database (Name) rather than a shared database selected via
// connection search_path, so there is no search_path query parameter
// to assert here.
func TestConfigDSN_ReflectsEveryField(t *testing.T) {
	cfg := postgres.Config{
		Host:     "db.internal",
		Port:     "6543",
		Name:     "appdb",
		User:     "appuser",
		Password: "s3cret-pw",
		SSLMode:  "require",
	}

	dsn := cfg.DSN()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(DSN()) unexpected error: %v", err)
	}
	if u.Scheme != "postgres" {
		t.Errorf("DSN() scheme = %q, want %q", u.Scheme, "postgres")
	}
	if u.Hostname() != cfg.Host {
		t.Errorf("DSN() host = %q, want %q", u.Hostname(), cfg.Host)
	}
	if u.Port() != cfg.Port {
		t.Errorf("DSN() port = %q, want %q", u.Port(), cfg.Port)
	}
	if got, want := strings.TrimPrefix(u.Path, "/"), cfg.Name; got != want {
		t.Errorf("DSN() path = %q, want %q", got, want)
	}
	if u.User.Username() != cfg.User {
		t.Errorf("DSN() user = %q, want %q", u.User.Username(), cfg.User)
	}
	if pw, _ := u.User.Password(); pw != cfg.Password {
		t.Errorf("DSN() password = %q, want %q", pw, cfg.Password)
	}
	if got := u.Query().Get("sslmode"); got != cfg.SSLMode {
		t.Errorf("DSN() sslmode = %q, want %q", got, cfg.SSLMode)
	}
}

// TestConfigValidate_MissingRequired asserts that Validate rejects a
// Config missing required fields (DB_NAME/DB_USER, with DB_HOST
// otherwise set) by naming exactly the missing variables, rather than
// letting a caller silently build a malformed DSN.
func TestConfigValidate_MissingRequired(t *testing.T) {
	cfg := postgres.Config{
		Host:     "db.internal",
		Password: "pw",
		// Name, User deliberately empty.
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error naming the missing DB_NAME/DB_USER")
	}
	for _, want := range []string{"DB_NAME", "DB_USER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "DB_HOST") {
		t.Errorf("Validate() error = %q, unexpectedly names DB_HOST, which was set", err.Error())
	}
}

// TestConfigValidate_ErrorNeverLeaksPassword is the R6 security
// assertion for Validate: when validation fails, the returned error
// lists only missing variable *names* -- never DB_PASSWORD's actual
// value, even though Password was supplied alongside missing required
// fields.
func TestConfigValidate_ErrorNeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-s3cret-db-password"
	cfg := postgres.Config{
		Host:     "db.internal",
		Password: secret,
		// Name, User deliberately empty.
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error (Name/User missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Validate() error %q leaks the DB_PASSWORD value", err.Error())
	}
	for _, want := range []string{"DB_NAME", "DB_USER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
}

// TestConfigValidate_AllRequiredFieldsPresent is the 正常系 case: a
// Config carrying every required field passes Validate with no error.
func TestConfigValidate_AllRequiredFieldsPresent(t *testing.T) {
	cfg := postgres.Config{
		Host:     "db.internal",
		Name:     "appdb",
		User:     "appuser",
		Password: "pw",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

// TestConfigEquality_ReaderWriterSharing pins the plain `==` comparison
// SPEC-010's postgres.OpenPair(ctx, writerCfg, readerCfg) relies on to
// decide whether the reader falls back to sharing the writer's single
// *sql.DB pool (readerCfg == writerCfg) or opens a second one
// (docs/plans/SPEC-010-plan.md "infra/postgres: OpenPair"). Config's
// fields are all plain strings, so this is Go's ordinary comparable-
// struct equality; this test exists to document and pin that
// assumption against any future non-comparable field being added to
// Config (which would make Config no longer usable with `==` and
// silently break OpenPair's sharing decision at compile time).
func TestConfigEquality_ReaderWriterSharing(t *testing.T) {
	base := postgres.Config{
		Host: "db.internal", Port: "5432", Name: "appdb",
		User: "appuser", Password: "pw", SSLMode: "require",
	}

	t.Run("identical field values compare equal", func(t *testing.T) {
		other := base
		if base != other {
			t.Errorf("Config{%+v} != Config{%+v}, want equal", base, other)
		}
	})

	tests := []struct {
		name   string
		mutate func(c postgres.Config) postgres.Config
	}{
		{"differing Host", func(c postgres.Config) postgres.Config { c.Host = "replica.internal"; return c }},
		{"differing Port", func(c postgres.Config) postgres.Config { c.Port = "6543"; return c }},
		{"differing Name", func(c postgres.Config) postgres.Config { c.Name = "otherdb"; return c }},
		{"differing User", func(c postgres.Config) postgres.Config { c.User = "otheruser"; return c }},
		{"differing Password", func(c postgres.Config) postgres.Config { c.Password = "other-pw"; return c }},
		{"differing SSLMode", func(c postgres.Config) postgres.Config { c.SSLMode = "disable"; return c }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := tt.mutate(base)
			if base == other {
				t.Errorf("Config{%+v} == Config{%+v}, want not equal (%s)", base, other, tt.name)
			}
		})
	}
}
