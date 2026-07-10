// This file is the single place app/auth reads the process environment
// (os.Getenv). Every other file in this composition root -- and every
// downstream package -- receives configuration as plain values (an
// Env or a postgres.Config), never by reading os.Getenv itself.
package main

import (
	"fmt"
	"os"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
)

// defaultSSLMode is fail-closed ("require"): if DB_SSLMODE is left
// unset, the connection defaults to encrypted rather than silently
// falling back to a plaintext one (ISSUE-016 m-2). Local development
// against a non-TLS Postgres (e.g. compose's postgres service) must
// set DB_SSLMODE=disable explicitly (see compose.yml and this repo's
// Makefiles, which already do so).
const (
	defaultPort    = "8080"
	defaultIssuer  = "http://localhost:8080"
	defaultDBPort  = "5432"
	defaultSSLMode = "require"
)

// Env holds every configuration value app/auth reads from the process
// environment. It is constructed once by NewEnv and passed downstream
// by value.
//
// Env has no DBSchema field (removed by the 2026-07-09 refactor,
// SPEC-005 plan §RF.2.3): auth now connects to its own dedicated
// Postgres database (DBName) instead of a shared database selected via
// connection search_path, so there is no DB_SCHEMA left to read.
type Env struct {
	Port   string
	AppEnv string
	Issuer string

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string
}

// NewEnv reads every environment variable app/auth depends on and
// applies defaults where applicable. It performs no validation --
// callers MUST call Env.validate before using DB_* values (see its
// doc comment for the fail-closed mode-dependent contract).
func NewEnv() Env {
	return Env{
		Port:   orDefault(os.Getenv("PORT"), defaultPort),
		AppEnv: os.Getenv("APP_ENV"),
		Issuer: orDefault(os.Getenv("ISSUER"), defaultIssuer),

		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     orDefault(os.Getenv("DB_PORT"), defaultDBPort),
		DBName:     os.Getenv("DB_NAME"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBSSLMode:  orDefault(os.Getenv("DB_SSLMODE"), defaultSSLMode),
	}
}

// dbConfig builds the postgres.Config carried by e's DB_* fields. It
// reads no environment itself; e is assumed to already be populated
// by NewEnv.
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

// validate resolves e's persistence mode and, only when that mode is
// Postgres, validates that the DB_* values required to connect are
// present -- DB_* is therefore required in Postgres mode and
// unconstrained otherwise (see postgres.SelectMode's fail-closed
// mode-selection contract). It returns the resolved Mode alongside
// any error so callers never need to call postgres.SelectMode a
// second time.
//
// The returned error never contains DB_PASSWORD's value (only var
// names may appear, via postgres.Config.Validate), and the %w chain
// preserves errors.Is(err, postgres.ErrPersistenceNotConfigured) so
// callers can distinguish "not configured" from other failures.
func (e Env) validate() (postgres.Mode, error) {
	mode, err := postgres.SelectMode(e.DBHost, e.AppEnv)
	if err != nil {
		return "", fmt.Errorf("authz: validate env: %w", err)
	}

	if mode == postgres.ModePostgres {
		if err := e.dbConfig().Validate(); err != nil {
			return "", fmt.Errorf("authz: validate env: %w", err)
		}
	}

	return mode, nil
}

// orDefault returns v, or def if v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
