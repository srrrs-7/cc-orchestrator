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
//
// Env has no AppEnv field (removed by SPEC-011): memory fallback is
// gone and fail-closed is enforced by Config.Validate (DB_HOST/DB_NAME/
// DB_USER/DB_PASSWORD required). APP_ENV is no longer consumed.
type Env struct {
	Port       string
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	// AuthIssuer and AuthJWKSURL configure Bearer JWT authentication.
	// When both are set, the API validates incoming JWTs against the
	// auth server's JWKS. When either is unset, auth middleware is
	// disabled (useful for local development without an auth server).
	// In production both must be set:
	//   AUTH_ISSUER   — expected iss/aud value in access tokens
	//   AUTH_JWKS_URL — URL of the auth server's JWKS endpoint
	AuthIssuer  string
	AuthJWKSURL string

	// DBReader holds the reader-pool connection settings SPEC-010
	// adds (docs/plans/SPEC-010-plan.md). Each field already carries
	// its *effective* value by the time NewEnv returns: NewEnv falls
	// each unset DB_READER_* item back to the corresponding writer
	// DB_* value above (R3/R4), so DBReader never needs re-resolving
	// downstream. When every DB_READER_* is left unset, DBReader ends
	// up field-for-field identical to the writer fields, which is
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

// NewEnv reads every environment variable app/api consumes and applies
// defaults where applicable. It performs no validation; call
// Env.validate to check the result.
func NewEnv() Env {
	e := Env{
		Port:        orDefault(os.Getenv("PORT"), defaultPort),
		DBHost:      os.Getenv("DB_HOST"),
		DBPort:      orDefault(os.Getenv("DB_PORT"), defaultDBPort),
		DBName:      os.Getenv("DB_NAME"),
		DBUser:      os.Getenv("DB_USER"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBSSLMode:   orDefault(os.Getenv("DB_SSLMODE"), defaultSSLMode),
		AuthIssuer:  os.Getenv("AUTH_ISSUER"),
		AuthJWKSURL: os.Getenv("AUTH_JWKS_URL"),
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

// writerConfig projects the already-captured writer-side DB_* values
// into postgres.Config. It reads no environment variables itself.
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

// readerConfig projects the already-captured (and already-fallen-back,
// see NewEnv) DBReader values into postgres.Config. It reads no
// environment variables itself. When every DB_READER_* was left unset,
// readerConfig() == writerConfig() field-for-field, which is the
// equality postgres.OpenPair relies on to share a single *sql.DB pool
// instead of opening a second one (SPEC-010 non-functional requirement:
// 二重に開かない).
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

// validate checks that the required DB_* variables are present for
// both the writer and reader configs (SPEC-011: Postgres is the only
// persistence backend; fail-closed is enforced by Config.Validate).
// It also enforces that AUTH_ISSUER and AUTH_JWKS_URL are either both
// set or both unset (partial auth config is a misconfiguration).
// The writer and reader configs are validated independently so that a
// partial DB_READER_* override (e.g. DB_READER_USER set to an empty
// string via a future non-Getenv-backed Env literal) is still caught.
// The returned error never contains DBPassword's or DBReader.Password's
// value.
func (e Env) validate() error {
	if err := e.writerConfig().Validate(); err != nil {
		return fmt.Errorf("api: validate env: %w", err)
	}
	if err := e.readerConfig().Validate(); err != nil {
		return fmt.Errorf("api: validate env: %w", err)
	}
	if (e.AuthIssuer == "") != (e.AuthJWKSURL == "") {
		return fmt.Errorf("api: validate env: AUTH_ISSUER and AUTH_JWKS_URL must both be set or both be unset")
	}
	return nil
}

// authEnabled reports whether JWT auth middleware should be activated.
// Both AUTH_ISSUER and AUTH_JWKS_URL must be non-empty (validate
// guarantees they are set together or not at all).
func (e Env) authEnabled() bool {
	return e.AuthIssuer != "" && e.AuthJWKSURL != ""
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
