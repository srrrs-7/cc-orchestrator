// persistence_selection_test.go exercises the pure connection-config
// helpers in infra/postgres that are DB-independent (SPEC-005 R6 /
// SPEC-011 R2):
//
//   - Config.DSN() reflects every field
//   - Config.Validate() rejects missing required fields by name (never
//     by value, so DB_PASSWORD is never echoed) and accepts a complete
//     Config without error
//   - Config is an ordinary comparable struct, so plain "==" works for
//     SPEC-010's OpenPair pool-sharing decision
//
// Persistence is now Postgres-only (SPEC-011): Mode / SelectMode /
// infra/memory fallback are removed. fail-closed is enforced by
// Config.Validate (DB_HOST/DB_NAME/DB_USER/DB_PASSWORD required).
//
// This file carries no build tag: DSN assembly and config validation
// are pure, DB-independent decisions, so it runs as part of the
// default `make test` and requires no live Postgres.
package postgres_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
)

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
