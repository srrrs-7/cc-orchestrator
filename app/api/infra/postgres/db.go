package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	// Blank-imported so importing this package registers "pgx" as a
	// database/sql driver name (used by Open below). This is the only
	// pgx symbol infra/postgres depends on directly; all query
	// execution goes through database/sql via infra/postgres/sqlcgen
	// (sqlc.yaml: sql_package: database/sql), keeping the generated
	// code itself standard-library-only (SPEC-005 non-functional
	// requirement).
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Mode identifies which task.Repository implementation
// cmd/api/main.go's persistence wiring block should construct.
type Mode string

const (
	// ModeMemory selects infra/memory.NewTaskRepository().
	ModeMemory Mode = "memory"
	// ModePostgres selects infra/postgres.NewTaskRepository(db).
	ModePostgres Mode = "postgres"
)

// pingTimeout bounds how long Open waits for the initial connectivity
// check at startup, independent of the caller's context (which, in
// cmd/api/main.go, is the long-lived signal.NotifyContext used for
// graceful shutdown -- Open must not block startup forever if that
// context happens to already be near cancellation).
const pingTimeout = 5 * time.Second

// Connection pool lifetime bounds applied by Open (SPEC-005 review E4,
// perf B-1). api's desired_count may be >1 (unlike auth, which is
// pinned to 1 -- see app/iac/modules/service/README.md "auth を
// desired_count = 1 に固定する理由"), and api/auth share one RDS
// instance, so an unbounded pool on this side alone can exhaust the
// shared instance's max_connections and take auth down with it. These
// values match app/auth/infra/postgres/db.go's Open exactly, kept as
// fixed, conservative defaults appropriate for this demo application's
// expected load rather than exposed as further configuration, to keep
// the persistence seam simple.
const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
)

// SelectMode decides between Postgres and the in-memory fallback,
// implementing SPEC-005 R6's fail-closed environment-selection
// contract (docs/plans/SPEC-005-plan.md §0 "切替の env / DSN / 本番必須強制"):
//
//	DB_HOST set                  -> ModePostgres, regardless of APP_ENV
//	DB_HOST unset, APP_ENV=local -> ModeMemory
//	DB_HOST unset, APP_ENV=test  -> ModeMemory
//	DB_HOST unset, otherwise     -> error (no memory fallback; this
//	                                 includes APP_ENV=production, an
//	                                 unset APP_ENV, and any unrecognized
//	                                 APP_ENV value)
//
// getenv is injected (rather than reading os.Getenv directly) so this
// pure decision can be unit-tested without mutating process-global
// environment state; cmd/api/main.go calls SelectMode(os.Getenv).
func SelectMode(getenv func(string) string) (Mode, error) {
	if getenv("DB_HOST") != "" {
		return ModePostgres, nil
	}

	switch appEnv := getenv("APP_ENV"); appEnv {
	case "local", "test":
		return ModeMemory, nil
	default:
		return "", fmt.Errorf(
			"postgres: select persistence mode: DB_HOST is not set and APP_ENV=%q does not permit the in-memory fallback (only \"local\" and \"test\" do); set DB_HOST to use Postgres, or APP_ENV=local/test to use the in-memory repository",
			appEnv,
		)
	}
}

// Config holds the discrete DB_* connection settings SPEC-005 R6
// standardizes on (docs/plans/SPEC-005-plan.md §0), as opposed to a
// single pre-assembled DSN/URL (so the password segment can come from
// a Secrets Manager-injected environment variable without iac ever
// having to compose a URL containing it).
type Config struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
	Schema   string
}

// ConfigFromEnv reads Config from the discrete DB_* environment
// variables, applying defaults for the ones that are safe to default
// (DB_PORT, DB_SSLMODE, DB_SCHEMA). DB_HOST/DB_NAME/DB_USER/DB_PASSWORD
// are read verbatim with no default, so DSN can reject a missing
// required value explicitly (see DSN's doc comment) instead of
// silently building a malformed connection string.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		Host:     getenv("DB_HOST"),
		Port:     orDefault(getenv("DB_PORT"), "5432"),
		Name:     getenv("DB_NAME"),
		User:     getenv("DB_USER"),
		Password: getenv("DB_PASSWORD"),
		SSLMode:  orDefault(getenv("DB_SSLMODE"), "disable"),
		// "api" matches db/migrations' unqualified DDL: the tasks table
		// is created in whichever schema search_path points at
		// (docs/plans/SPEC-005-plan.md §0 "スキーマ分離機構"), and "api" is
		// this stack's schema.
		Schema: orDefault(getenv("DB_SCHEMA"), "api"),
	}
}

// DSN validates cfg and assembles a libpq-style connection string
// (postgres://user:password@host:port/name?sslmode=...&search_path=...)
// suitable for sql.Open("pgx", ...). It returns an error listing the
// missing field name(s) when a required setting (DB_HOST/DB_NAME/
// DB_USER/DB_PASSWORD) is empty, rather than silently passing an empty
// string through to sql.Open. The error text never includes
// cfg.Password, so a fail-closed error is safe to log.
func (cfg Config) DSN() (string, error) {
	var missing []string
	if cfg.Host == "" {
		missing = append(missing, "DB_HOST")
	}
	if cfg.Name == "" {
		missing = append(missing, "DB_NAME")
	}
	if cfg.User == "" {
		missing = append(missing, "DB_USER")
	}
	if cfg.Password == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("postgres: build dsn: missing required settings: %s", strings.Join(missing, ", "))
	}

	query := url.Values{}
	query.Set("sslmode", cfg.SSLMode)
	query.Set("search_path", cfg.Schema)

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host + ":" + cfg.Port,
		Path:     "/" + cfg.Name,
		RawQuery: query.Encode(),
	}
	return dsn.String(), nil
}

// Open builds a DSN from cfg, opens a *sql.DB against it using the pgx
// stdlib driver, registers connection-pool lifetime bounds, and
// verifies connectivity with a bounded Ping before returning. The
// returned *sql.DB owns a connection pool whose lifetime the caller
// controls; cmd/api/main.go closes it via db.Close() during graceful
// shutdown. On any failure the returned error never includes
// cfg.Password (DSN's contract; sql.Open/PingContext errors from the
// pgx driver do not echo credentials either).
//
// Open does not tie the pool's lifetime to ctx: ctx only bounds the
// initial ping (via the fixed pingTimeout, not ctx's own possibly
// long-lived deadline), matching database/sql's own *sql.DB lifecycle
// model (a *sql.DB is a long-lived pool handle, not itself
// context-scoped).
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	dsn, err := cfg.DSN()
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: open: ping: %w", err)
	}

	return db, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
