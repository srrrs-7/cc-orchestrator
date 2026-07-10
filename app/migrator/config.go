package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config holds the discrete DB_* connection settings this migrator
// reads from the environment. Unlike the pre-refactor
// infra/postgres.Config in app/api and app/auth, Config has no
// Schema/search_path field: SPEC-005's 2026-07-09 refactor (plan
// §RF.1.1) replaces schema separation with one dedicated Postgres
// database per stack, so there is no search_path to select.
//
// Name is the target database this migrator creates (if missing) and
// migrates. MaintenanceName is the database CREATE DATABASE is issued
// against, since CREATE DATABASE cannot target the connection's own
// database (see ensureDatabase in database.go).
type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	SSLMode         string
	Name            string
	MaintenanceName string
}

// Defaults for the DB_* settings that are safe to default. DB_HOST/
// DB_USER/DB_PASSWORD have no default and are validated as required by
// configFromEnv, matching app/{api,auth}/infra/postgres.ConfigFromEnv's
// existing contract for those three variables.
//
// defaultSSLMode is fail-closed ("require"): if DB_SSLMODE is left
// unset, the connection defaults to encrypted rather than silently
// falling back to a plaintext one (ISSUE-016 m-2). This matters most
// here, since this migrator connects with the master credentials to
// run CREATE DATABASE / goose migrations -- the highest-privilege
// connection in the stack. Local development against a non-TLS
// Postgres (e.g. compose's postgres service) must set
// DB_SSLMODE=disable explicitly (see the root Makefile's
// MIGRATOR_DB_ENV, which already does so).
const (
	defaultPort            = "5432"
	defaultSSLMode         = "require"
	defaultMaintenanceName = "postgres"
)

// configFromEnv reads the discrete DB_* environment variables
// (DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_SSLMODE/DB_NAME/
// DB_MAINTENANCE_NAME) for the given target ("api" or "auth"),
// defaulting DB_NAME to target itself (plan §RF.1.1: DB name per
// service = "api"/"auth") when unset. A missing required variable is
// reported by name only; the returned error, like
// app/{api,auth}/infra/postgres.ConfigFromEnv's, never echoes
// DB_PASSWORD's value.
func configFromEnv(target string) (Config, error) {
	cfg := Config{
		Host:            os.Getenv("DB_HOST"),
		Port:            envOrDefault("DB_PORT", defaultPort),
		User:            os.Getenv("DB_USER"),
		Password:        os.Getenv("DB_PASSWORD"),
		SSLMode:         envOrDefault("DB_SSLMODE", defaultSSLMode),
		Name:            envOrDefault("DB_NAME", target),
		MaintenanceName: envOrDefault("DB_MAINTENANCE_NAME", defaultMaintenanceName),
	}

	var missing []string
	if cfg.Host == "" {
		missing = append(missing, "DB_HOST")
	}
	if cfg.User == "" {
		missing = append(missing, "DB_USER")
	}
	if cfg.Password == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("config from env: missing required variable(s): %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// dsn assembles a libpq-style connection string against dbName --
// either cfg.MaintenanceName (phase 1: ensureDatabase's CREATE
// DATABASE bootstrap) or cfg.Name (phase 2: the goose migration run
// itself), depending on which run() calls with. It never sets a
// search_path query parameter (plan §RF.1.1: database separation
// replaces schema separation, so there is nothing to select). The
// returned string embeds cfg.Password in cleartext, as any connection
// string must; callers MUST NOT log it (run() never does).
func (cfg Config) dsn(dbName string) string {
	values := url.Values{}
	values.Set("sslmode", cfg.SSLMode)

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host + ":" + cfg.Port,
		Path:     "/" + dbName,
		RawQuery: values.Encode(),
	}
	return u.String()
}
