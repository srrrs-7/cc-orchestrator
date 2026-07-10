package main

import (
	"net/url"
	"strings"
	"testing"
)

// setAllDBEnv sets every DB_* variable configFromEnv reads to an
// explicit value for the duration of the calling test (t.Setenv
// auto-restores on cleanup), so each subtest starts from a fully known
// environment instead of inheriting whatever the process happened to
// have set (SPEC-005 plan §RF.6.2 RB2: table-driven, no reliance on
// execution order). Setting a variable to "" is equivalent to leaving
// it unset for every consumer in this package (they all check
// os.Getenv(...) == "").
func setAllDBEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD",
		"DB_SSLMODE", "DB_NAME", "DB_MAINTENANCE_NAME",
	} {
		t.Setenv(key, env[key])
	}
}

// TestConfigFromEnv_DefaultDatabaseNamePerTarget is the R5/§RF.1.1
// "DB_NAME defaults to target itself" contract: with DB_NAME unset,
// api gets database "api" and auth gets database "auth" -- the two
// stacks are separated by database, not by a shared schema/search_path
// selector (that concept was removed by the 2026-07-09 refactor).
func TestConfigFromEnv_DefaultDatabaseNamePerTarget(t *testing.T) {
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

			cfg, err := configFromEnv(tc.target)
			if err != nil {
				t.Fatalf("configFromEnv(%q) unexpected error: %v", tc.target, err)
			}
			if cfg.Name != tc.wantDBName {
				t.Errorf("configFromEnv(%q).Name = %q, want %q", tc.target, cfg.Name, tc.wantDBName)
			}
		})
	}
}

// TestConfigFromEnv_DBNameExplicitOverride asserts an explicit DB_NAME
// wins over the target-derived default.
func TestConfigFromEnv_DBNameExplicitOverride(t *testing.T) {
	setAllDBEnv(t, map[string]string{
		"DB_HOST":     "db.internal",
		"DB_USER":     "app",
		"DB_PASSWORD": "pw",
		"DB_NAME":     "custom_db",
	})

	cfg, err := configFromEnv("api")
	if err != nil {
		t.Fatalf("configFromEnv() unexpected error: %v", err)
	}
	if cfg.Name != "custom_db" {
		t.Errorf("configFromEnv().Name = %q, want %q (explicit DB_NAME must win over the api default)", cfg.Name, "custom_db")
	}
}

// TestConfigFromEnv_Defaults covers the remaining defaulted fields:
// DB_PORT, DB_SSLMODE and DB_MAINTENANCE_NAME. DB_SSLMODE's default is
// fail-closed ("require", ISSUE-016 m-2): omitting it must not
// silently downgrade this migrator's (master-credentialed) connection
// to plaintext.
func TestConfigFromEnv_Defaults(t *testing.T) {
	setAllDBEnv(t, map[string]string{
		"DB_HOST":     "db.internal",
		"DB_USER":     "app",
		"DB_PASSWORD": "pw",
	})

	cfg, err := configFromEnv("api")
	if err != nil {
		t.Fatalf("configFromEnv() unexpected error: %v", err)
	}
	if cfg.Port != "5432" {
		t.Errorf("configFromEnv().Port = %q, want default %q", cfg.Port, "5432")
	}
	if cfg.SSLMode != "require" {
		t.Errorf("configFromEnv().SSLMode = %q, want default %q", cfg.SSLMode, "require")
	}
	if cfg.MaintenanceName != "postgres" {
		t.Errorf("configFromEnv().MaintenanceName = %q, want default %q", cfg.MaintenanceName, "postgres")
	}
}

// TestConfigFromEnv_MaintenanceNameExplicitOverride covers
// DB_MAINTENANCE_NAME's escape hatch (plan §RF.6.1 RF-a: some RDS
// deployments may not expose a usable "postgres" database).
func TestConfigFromEnv_MaintenanceNameExplicitOverride(t *testing.T) {
	setAllDBEnv(t, map[string]string{
		"DB_HOST":             "db.internal",
		"DB_USER":             "app",
		"DB_PASSWORD":         "pw",
		"DB_MAINTENANCE_NAME": "bootstrap",
	})

	cfg, err := configFromEnv("auth")
	if err != nil {
		t.Fatalf("configFromEnv() unexpected error: %v", err)
	}
	if cfg.MaintenanceName != "bootstrap" {
		t.Errorf("configFromEnv().MaintenanceName = %q, want %q", cfg.MaintenanceName, "bootstrap")
	}
}

// TestConfigFromEnv_MissingRequired is the fail-closed contract for
// the three variables with no default (DB_HOST/DB_USER/DB_PASSWORD,
// mirroring app/{api,auth}/infra/postgres.Config.Validate): a missing
// required variable is reported by name, and the error never echoes
// DB_PASSWORD's value even when it was one of the fields present.
func TestConfigFromEnv_MissingRequired(t *testing.T) {
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

			_, err := configFromEnv("api")
			if err == nil {
				t.Fatalf("configFromEnv() = nil error, want an error naming %v", tc.want)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("configFromEnv() error = %q, want it to mention missing variable %s", err.Error(), want)
				}
			}
			for _, notWant := range tc.mustNot {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("configFromEnv() error = %q, unexpectedly names %s, which was set", err.Error(), notWant)
				}
			}
		})
	}
}

// TestConfigFromEnv_MissingRequiredNeverLeaksPassword is the R6
// security assertion: when DB_PASSWORD is set to a secret value but
// another required variable is missing, the resulting error must never
// contain that secret value.
func TestConfigFromEnv_MissingRequiredNeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-s3cret-migrator-password"
	setAllDBEnv(t, map[string]string{
		"DB_USER":     "app",
		"DB_PASSWORD": secret,
		// DB_HOST deliberately left unset so configFromEnv errors.
	})

	_, err := configFromEnv("api")
	if err == nil {
		t.Fatal("configFromEnv() = nil error, want an error (DB_HOST missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("configFromEnv() error %q leaks the DB_PASSWORD value", err.Error())
	}
}

// TestConfig_DSN_NoSearchPath asserts dsn() never sets a search_path
// query parameter, the 2026-07-09 refactor's central DSN change (plan
// §RF.1.1: database separation, not schema/search_path selection).
func TestConfig_DSN_NoSearchPath(t *testing.T) {
	cfg := Config{
		Host:     "db.internal",
		Port:     "6543",
		User:     "appuser",
		Password: "s3cret-pw",
		SSLMode:  "require",
	}

	dsn := cfg.dsn("api")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(dsn) unexpected error: %v", err)
	}
	if u.Query().Has("search_path") {
		t.Errorf("dsn() unexpectedly sets search_path=%q", u.Query().Get("search_path"))
	}
}

// TestConfig_DSN_ReflectsEveryField asserts every Config field is
// represented in the DSN dsn(dbName) assembles, and that dbName (not
// cfg.Name) selects the path -- the mechanism run() relies on to
// connect to cfg.MaintenanceName first and cfg.Name second (main.go's
// ensureTargetDatabase / run doc comments).
func TestConfig_DSN_ReflectsEveryField(t *testing.T) {
	cfg := Config{
		Host:     "db.internal",
		Port:     "6543",
		User:     "appuser",
		Password: "s3cret-pw",
		SSLMode:  "require",
		Name:     "api",
	}

	dsn := cfg.dsn("some_other_db")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(dsn) unexpected error: %v", err)
	}
	if u.Scheme != "postgres" {
		t.Errorf("dsn() scheme = %q, want %q", u.Scheme, "postgres")
	}
	if u.Hostname() != cfg.Host {
		t.Errorf("dsn() host = %q, want %q", u.Hostname(), cfg.Host)
	}
	if u.Port() != cfg.Port {
		t.Errorf("dsn() port = %q, want %q", u.Port(), cfg.Port)
	}
	if got, want := strings.TrimPrefix(u.Path, "/"), "some_other_db"; got != want {
		t.Errorf("dsn() path = %q, want %q (the dbName argument, not cfg.Name)", got, want)
	}
	if u.User.Username() != cfg.User {
		t.Errorf("dsn() user = %q, want %q", u.User.Username(), cfg.User)
	}
	if pw, _ := u.User.Password(); pw != cfg.Password {
		t.Errorf("dsn() password = %q, want %q", pw, cfg.Password)
	}
	if got := u.Query().Get("sslmode"); got != cfg.SSLMode {
		t.Errorf("dsn() sslmode = %q, want %q", got, cfg.SSLMode)
	}
}

// TestConfig_DSN_SelectsMaintenanceOrTargetName is the two-phase
// connection contract run()/ensureTargetDatabase rely on: dsn(dbName)
// must reflect whichever database name is passed, independent of
// cfg.Name/cfg.MaintenanceName themselves (those fields are read by
// the caller, not by dsn).
func TestConfig_DSN_SelectsMaintenanceOrTargetName(t *testing.T) {
	cfg := Config{
		Host:            "db.internal",
		Port:            "5432",
		User:            "app",
		Password:        "pw",
		SSLMode:         "disable",
		Name:            "api",
		MaintenanceName: "postgres",
	}

	maintenanceDSN, err := url.Parse(cfg.dsn(cfg.MaintenanceName))
	if err != nil {
		t.Fatalf("url.Parse(dsn(MaintenanceName)) unexpected error: %v", err)
	}
	if got := strings.TrimPrefix(maintenanceDSN.Path, "/"); got != "postgres" {
		t.Errorf("dsn(cfg.MaintenanceName) path = %q, want %q", got, "postgres")
	}

	targetDSN, err := url.Parse(cfg.dsn(cfg.Name))
	if err != nil {
		t.Fatalf("url.Parse(dsn(Name)) unexpected error: %v", err)
	}
	if got := strings.TrimPrefix(targetDSN.Path, "/"); got != "api" {
		t.Errorf("dsn(cfg.Name) path = %q, want %q", got, "api")
	}
}
