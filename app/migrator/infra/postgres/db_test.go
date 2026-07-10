package postgres

import (
	"net/url"
	"strings"
	"testing"
)

// TestConfig_DSN_NoSearchPath asserts DSN never sets a search_path
// query parameter, the 2026-07-09 refactor's central DSN change
// (database separation, not schema/search_path selection). Moved from
// the pre-refactor config_test.go's TestConfig_DSN_NoSearchPath.
func TestConfig_DSN_NoSearchPath(t *testing.T) {
	cfg := Config{
		Host:     "db.internal",
		Port:     "6543",
		User:     "appuser",
		Password: "s3cret-pw",
		SSLMode:  "require",
	}

	dsn := cfg.DSN("api")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(dsn) unexpected error: %v", err)
	}
	if u.Query().Has("search_path") {
		t.Errorf("DSN() unexpectedly sets search_path=%q", u.Query().Get("search_path"))
	}
}

// TestConfig_DSN_ReflectsEveryField asserts every Config field is
// represented in the DSN DSN(dbName) assembles, and that dbName
// selects the path -- the mechanism infra/postgres.EnsureExister and
// infra/goose.Runner rely on to connect to cfg.MaintenanceName and the
// target database respectively. Moved from the pre-refactor
// config_test.go's TestConfig_DSN_ReflectsEveryField.
func TestConfig_DSN_ReflectsEveryField(t *testing.T) {
	cfg := Config{
		Host:     "db.internal",
		Port:     "6543",
		User:     "appuser",
		Password: "s3cret-pw",
		SSLMode:  "require",
	}

	dsn := cfg.DSN("some_other_db")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(dsn) unexpected error: %v", err)
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
	if got, want := strings.TrimPrefix(u.Path, "/"), "some_other_db"; got != want {
		t.Errorf("DSN() path = %q, want %q (the dbName argument)", got, want)
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

// TestConfig_DSN_SelectsMaintenanceOrTargetName is the two-connection
// contract infra/postgres.EnsureExister/infra/goose.Runner rely on:
// DSN(dbName) must reflect whichever database name is passed,
// independent of cfg.MaintenanceName itself (that field is read by
// the caller, not by DSN). Moved from the pre-refactor config_test.go's
// TestConfig_DSN_SelectsMaintenanceOrTargetName.
func TestConfig_DSN_SelectsMaintenanceOrTargetName(t *testing.T) {
	cfg := Config{
		Host:            "db.internal",
		Port:            "5432",
		User:            "app",
		Password:        "pw",
		SSLMode:         "disable",
		MaintenanceName: "postgres",
	}

	maintenanceDSN, err := url.Parse(cfg.DSN(cfg.MaintenanceName))
	if err != nil {
		t.Fatalf("url.Parse(DSN(MaintenanceName)) unexpected error: %v", err)
	}
	if got := strings.TrimPrefix(maintenanceDSN.Path, "/"); got != "postgres" {
		t.Errorf("DSN(cfg.MaintenanceName) path = %q, want %q", got, "postgres")
	}

	targetDSN, err := url.Parse(cfg.DSN("api"))
	if err != nil {
		t.Fatalf("url.Parse(DSN(\"api\")) unexpected error: %v", err)
	}
	if got := strings.TrimPrefix(targetDSN.Path, "/"); got != "api" {
		t.Errorf("DSN(\"api\") path = %q, want %q", got, "api")
	}
}
