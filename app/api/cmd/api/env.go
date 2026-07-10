// This file is the single place app/api reads the process environment.
// No other file in this module should call os.Getenv; everything else
// receives already-captured values via Env or postgres.Config.
package main

import (
	"fmt"
	"os"

	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
)

// defaultSSLMode is fail-closed ("require"): if DB_SSLMODE is left
// unset, the connection defaults to encrypted rather than silently
// falling back to a plaintext one (ISSUE-016 m-2). Local development
// against a non-TLS Postgres (e.g. compose's postgres service) must
// set DB_SSLMODE=disable explicitly (see compose.yml and this repo's
// Makefiles, which already do so).
const (
	defaultPort    = "8080"
	defaultDBPort  = "5432"
	defaultSSLMode = "require"
)

// Env holds every environment-derived value app/api needs at startup.
//
// Env has no DBSchema field (removed by the 2026-07-09 refactor,
// SPEC-005 plan §RF.2.2): api now connects to its own dedicated
// Postgres database (DBName) instead of a shared database selected via
// connection search_path, so there is no DB_SCHEMA left to read.
type Env struct {
	Port       string
	AppEnv     string
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string
}

// NewEnv reads every environment variable app/api consumes and applies
// defaults where applicable. It performs no validation; call
// Env.validate to check the result.
func NewEnv() Env {
	return Env{
		Port:       orDefault(os.Getenv("PORT"), defaultPort),
		AppEnv:     os.Getenv("APP_ENV"),
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     orDefault(os.Getenv("DB_PORT"), defaultDBPort),
		DBName:     os.Getenv("DB_NAME"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBSSLMode:  orDefault(os.Getenv("DB_SSLMODE"), defaultSSLMode),
	}
}

// dbConfig projects the already-captured DB_* values into
// postgres.Config. It reads no environment variables itself.
func (e Env) dbConfig() postgres.Config {
	return postgres.Config{
		Host:     e.DBHost,
		Port:     e.DBPort,
		Name:     e.DBName,
		User:     e.DBUser,
		Password: e.DBPassword,
		SSLMode:  e.DBSSLMode,
	}
}

// validate resolves the persistence mode and, when that mode is
// Postgres, checks that the required DB_* variables are present
// (DB_* are optional in memory mode). It returns the resolved Mode so
// callers need not call postgres.SelectMode a second time. The
// returned error never contains DBPassword's value.
func (e Env) validate() (postgres.Mode, error) {
	mode, err := postgres.SelectMode(e.DBHost, e.AppEnv)
	if err != nil {
		return "", fmt.Errorf("api: validate env: %w", err)
	}
	if mode == postgres.ModePostgres {
		if err := e.dbConfig().Validate(); err != nil {
			return "", fmt.Errorf("api: validate env: %w", err)
		}
	}
	return mode, nil
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
