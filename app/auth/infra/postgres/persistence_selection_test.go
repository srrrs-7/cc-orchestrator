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
// env.go refactor note: postgres.ConfigFromEnv is removed entirely --
// cmd/authz's env.go (package main) is now the sole place this module
// reads os.Getenv (see cmd/authz/env_test.go for that coverage,
// including the defaults previously asserted here via ConfigFromEnv).
// postgres.SelectMode's signature is unchanged (it already took the
// two already-read string values, dbHost/appEnv, directly rather than
// reading os.Getenv itself), so those tests are kept as-is below. This
// file now covers only postgres.SelectMode and the pure
// postgres.Config methods (Validate/DSN), built from explicit literals
// instead of the removed setDBEnv/t.Setenv helper.
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

// TestConfigDSN_ReflectsEveryField asserts every field of a
// fully-populated postgres.Config is represented in the DSN string
// (postgres.Config).DSN() assembles from it (order-independent field
// checks via net/url, not a fixed string layout), and that no
// search_path parameter is set.
//
// Config carries no Schema field (removed by the 2026-07-09 refactor,
// SPEC-005 plan §RF.2.3): auth now connects to its own dedicated
// Postgres database (Name) rather than a shared database selected via
// connection search_path.
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
	if u.Query().Has("search_path") {
		t.Errorf("DSN() unexpectedly sets search_path=%q (R3/RF.1.1: database separation replaces schema separation)", u.Query().Get("search_path"))
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
