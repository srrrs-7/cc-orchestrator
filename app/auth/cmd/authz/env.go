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
	defaultPort        = "8080"
	defaultIssuer      = "http://localhost:8080"
	defaultAPIAudience = "http://localhost:8081/api"
	defaultDBPort      = "5432"
	defaultSSLMode     = "require"
)

// Env holds every configuration value app/auth reads from the process
// environment. It is constructed once by NewEnv and passed downstream
// by value.
//
// Env has no AppEnv field (removed by SPEC-011: infra/memory is
// deleted; Postgres is the sole persistence backend and APP_ENV is
// no longer consulted for mode selection). Env has no DBSchema field
// (removed by the 2026-07-09 refactor, SPEC-005 plan §RF.2.3): auth
// now connects to its own dedicated Postgres database (DBName) instead
// of a shared database selected via connection search_path.
type Env struct {
	Port   string
	Issuer string

	// APIAudience is the resource identifier placed in the "aud" claim of
	// issued access tokens (ISSUE-037). app/api verifies its AUTH_AUDIENCE
	// against this value. Defaults to defaultAPIAudience when unset.
	APIAudience string

	// SigningKeysFile is the path to the JSON key ring file produced by
	// `make auth-signing-keys` (ISSUE-036). When empty, the process
	// generates an ephemeral RSA key pair that does not survive restart
	// (acceptable for local development, not for production).
	SigningKeysFile string

	DemoPassword string

	// AdminAPIKey is the static Bearer token / X-Admin-Key value that
	// protects the /admin/* routes (ISSUE-039). When empty, no admin
	// routes are registered (fail-closed: missing key → no access).
	AdminAPIKey string

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	// DBReader holds the reader-pool connection settings SPEC-010
	// adds (docs/plans/SPEC-010-plan.md, symmetric with app/api's
	// cmd/api/env.go). Each field already carries its *effective*
	// value by the time NewEnv returns: NewEnv falls each unset
	// DB_READER_* item back to the corresponding writer DB_* value
	// above (R3/R4), so DBReader never needs re-resolving downstream.
	// When every DB_READER_* is left unset, DBReader ends up
	// field-for-field identical to the writer fields, which is
	// exactly the equality (Env).readerConfig() == (Env).writerConfig()
	// relies on for postgres.OpenPair to share a single pool instead
	// of opening a second one (二重に開かない).
	DBReader DBReaderEnv
}

// DBReaderEnv mirrors Env's writer-side DB_* fields for the reader
// pool (SPEC-010 R4). It is symmetric with Env's own DBHost/DBPort/
// DBName/DBUser/DBPassword/DBSSLMode fields by design.
type DBReaderEnv struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

// NewEnv reads every environment variable app/auth depends on and
// applies defaults where applicable. It performs no validation --
// callers MUST call Env.validate before using DB_* values (see its
// doc comment for the fail-closed contract).
func NewEnv() Env {
	e := Env{
		Port:            orDefault(os.Getenv("PORT"), defaultPort),
		Issuer:          orDefault(os.Getenv("ISSUER"), defaultIssuer),
		APIAudience:     orDefault(os.Getenv("API_AUDIENCE"), defaultAPIAudience),
		SigningKeysFile: os.Getenv("SIGNING_KEYS_FILE"),
		DemoPassword:    os.Getenv("DEMO_PASSWORD"),
		AdminAPIKey:     os.Getenv("ADMIN_API_KEY"),

		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     orDefault(os.Getenv("DB_PORT"), defaultDBPort),
		DBName:     os.Getenv("DB_NAME"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBSSLMode:  orDefault(os.Getenv("DB_SSLMODE"), defaultSSLMode),
	}

	// SPEC-010 R3/R4: each DB_READER_* item falls back individually to
	// the writer's own (already-defaulted) value when unset, so a
	// partially-configured reader (e.g. only DB_READER_HOST set) still
	// yields a fully valid Config for the remaining fields.
	e.DBReader = DBReaderEnv{
		Host:     orDefault(os.Getenv("DB_READER_HOST"), e.DBHost),
		Port:     orDefault(os.Getenv("DB_READER_PORT"), e.DBPort),
		Name:     orDefault(os.Getenv("DB_READER_NAME"), e.DBName),
		User:     orDefault(os.Getenv("DB_READER_USER"), e.DBUser),
		Password: orDefault(os.Getenv("DB_READER_PASSWORD"), e.DBPassword),
		SSLMode:  orDefault(os.Getenv("DB_READER_SSLMODE"), e.DBSSLMode),
	}

	return e
}

// writerConfig projects e's writer-side DB_* fields into
// postgres.Config. It reads no environment itself; e is assumed to
// already be populated by NewEnv. Replaces the pre-SPEC-010 dbConfig.
func (e Env) writerConfig() postgres.Config {
	return postgres.Config{
		Host:     e.DBHost,
		Port:     e.DBPort,
		Name:     e.DBName,
		User:     e.DBUser,
		Password: e.DBPassword,
		SSLMode:  e.DBSSLMode,
	}
}

// readerConfig projects e's (already-captured and already-fallen-back,
// see NewEnv) DBReader fields into postgres.Config. It reads no
// environment itself. When every DB_READER_* was left unset,
// readerConfig() == writerConfig() field-for-field, which is the
// equality postgres.OpenPair relies on to share a single *sql.DB pool
// instead of opening a second one (SPEC-010 non-functional
// requirement: 二重に開かない).
func (e Env) readerConfig() postgres.Config {
	return postgres.Config{
		Host:     e.DBReader.Host,
		Port:     e.DBReader.Port,
		Name:     e.DBReader.Name,
		User:     e.DBReader.User,
		Password: e.DBReader.Password,
		SSLMode:  e.DBReader.SSLMode,
	}
}

// validate checks that the DB_* values required to open Postgres
// connections are present for both the writer and reader configs.
// Postgres is the sole persistence backend (SPEC-011): fail-closed is
// enforced here via Config.Validate, which requires DB_HOST / DB_NAME
// / DB_USER / DB_PASSWORD to be set. APP_ENV is no longer consulted
// (removed by SPEC-011 together with infra/memory).
//
// Validating the reader config too is mostly redundant once the
// writer config is valid, since NewEnv already fell every unset
// DB_READER_* item back to the (about-to-be-validated) writer value --
// but it still catches a partial, invalid override supplied via a
// hand-built Env literal (e.g. in a test).
//
// The returned error never contains DB_PASSWORD's or
// DBReader.Password's value (only var names may appear, via
// postgres.Config.Validate).
func (e Env) validate() error {
	if err := e.writerConfig().Validate(); err != nil {
		return fmt.Errorf("authz: validate env: %w", err)
	}
	if err := e.readerConfig().Validate(); err != nil {
		return fmt.Errorf("authz: validate env: %w", err)
	}
	return nil
}

// orDefault returns v, or def if v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
