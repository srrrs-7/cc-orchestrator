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
// SelectMode is a pure function of its two arguments: it never reads
// os.Getenv itself. cmd/api/main.go reads DB_HOST/APP_ENV and passes
// them in explicitly, which keeps this decision unit-testable without
// mutating process-global environment state.
func SelectMode(dbHost, appEnv string) (Mode, error) {
	if dbHost != "" {
		return ModePostgres, nil
	}

	switch appEnv {
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
//
// Config has no Schema field (removed by the 2026-07-09 refactor, plan
// §RF.2.2): api and auth now each connect to their own dedicated
// Postgres database (DB_NAME) instead of sharing one database
// distinguished by connection search_path, so there is nothing left to
// select via a schema/search_path setting. Database creation and
// migration are handled out of band by app/migrator, not this package
// (see .claude/rules/db.md).
type Config struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

// Validate reports a configuration error listing only the missing
// required field *names* (DB_HOST/DB_NAME/DB_USER/DB_PASSWORD; never
// any value, so a missing DB_PASSWORD is reported without ever
// printing a password) when cfg lacks a value required to open a
// connection, rather than letting DSN silently build a malformed
// connection string. Returns nil when cfg has every required field.
func (cfg Config) Validate() error {
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
		return fmt.Errorf("postgres: config: missing required settings: %s", strings.Join(missing, ", "))
	}
	return nil
}

// DSN assembles a libpq-style connection string
// (postgres://user:password@host:port/name?sslmode=...) suitable for
// sql.Open("pgx", ...). DSN performs no validation of its own (see
// Validate); callers that need a fail-closed check on missing required
// fields should call cfg.Validate() first, as Open does.
func (cfg Config) DSN() string {
	query := url.Values{}
	query.Set("sslmode", cfg.SSLMode)

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host + ":" + cfg.Port,
		Path:     "/" + cfg.Name,
		RawQuery: query.Encode(),
	}
	return dsn.String()
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
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	db, err := sql.Open("pgx", cfg.DSN())
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

// OpenPair opens the writer and reader connection pools SPEC-010's
// Reader/Writer split routes queries through: task.Writer
// implementations (TaskWriter) are wired to the returned writer pool,
// task.Reader implementations (TaskReader) to reader.
//
// When readerCfg equals writerCfg (Config is a plain struct of
// comparable string fields, so "==" is ordinary value equality --
// pinned by persistence_selection_test.go's
// TestConfigEquality_ReaderWriterSharing), the reader has nowhere
// distinct to connect to, so OpenPair opens exactly one pool (via
// Open, applying the same pool-size/ping-timeout bounds as every
// other caller of Open) and returns it as both writer and reader: the
// same *sql.DB pointer. This is the fail-safe default (SPEC-010 R3 /
// non-functional "二重に開かない") that keeps every existing
// deployment -- which sets no DB_READER_* -- on a single pool exactly
// as before this Spec.
//
// Only when readerCfg differs from writerCfg does OpenPair open a
// second, independent pool for the reader. If that second Open fails,
// OpenPair closes the already-opened writer pool before returning, so
// a failed OpenPair call never leaks the writer pool back to a caller
// that has no reference to it (the returned writer/reader/closeFn are
// all nil in that case).
//
// On success, closeFn releases every pool OpenPair actually opened,
// exactly once each: closing the shared pool once when reader ==
// writer, or closing both independent pools when they differ. Callers
// (cmd/api/main.go) should defer closeFn() instead of calling
// writer.Close()/reader.Close() directly, so they don't need to know
// whether the two pools happen to be the same one.
func OpenPair(ctx context.Context, writerCfg, readerCfg Config) (writer, reader *sql.DB, closeFn func() error, err error) {
	writer, err = Open(ctx, writerCfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("postgres: open pair: writer: %w", err)
	}

	if readerCfg == writerCfg {
		return writer, writer, writer.Close, nil
	}

	reader, err = Open(ctx, readerCfg)
	if err != nil {
		_ = writer.Close()
		return nil, nil, nil, fmt.Errorf("postgres: open pair: reader: %w", err)
	}

	closeFn = func() error {
		writerErr := writer.Close()
		readerErr := reader.Close()
		if writerErr != nil {
			return writerErr
		}
		return readerErr
	}
	return writer, reader, closeFn, nil
}
