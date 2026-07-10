// Package postgres is app/migrator's implementation of the
// migration.Database port (see database.go's EnsureExister), plus the
// shared low-level connection helpers (Config, Open) infra/goose also
// uses to open its own target-database connection. Compare
// app/{api,auth}/infra/postgres, which plays the equivalent role for
// those modules' own domain.Repository ports.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	// Blank-imported so importing this package registers "pgx" as a
	// database/sql driver name (used by Open below).
	_ "github.com/jackc/pgx/v5/stdlib"
)

// pingTimeout bounds how long Open waits for the initial connectivity
// check on each connection this module opens (the maintenance
// connection EnsureExister.EnsureExists uses, and the target
// connection infra/goose.Runner.Run uses), matching
// app/{api,auth}/infra/postgres/db.go's Open exactly.
const pingTimeout = 5 * time.Second

// Config holds the discrete DB_* connection settings this migrator's
// infra packages (this one and infra/goose) share: the connection
// itself (Host/Port/User/Password/SSLMode) plus MaintenanceName, the
// database CREATE DATABASE is issued against (CREATE DATABASE cannot
// target the connection's own database). Unlike
// app/{api,auth}/infra/postgres.Config, Config has no target database
// name field: a single migrator run touches two distinct databases
// (MaintenanceName, then the target) with one Config, so the database
// name is instead passed explicitly to DSN by each caller
// (EnsureExists uses MaintenanceName; infra/goose.Runner uses the
// migration.DatabaseName it was built with).
type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	SSLMode         string
	MaintenanceName string
}

// DSN assembles a libpq-style connection string
// (postgres://user:password@host:port/dbName?sslmode=...) against
// dbName. It never sets a search_path query parameter (2026-07-09
// refactor: per-stack database separation replaces schema
// separation, so there is nothing to select). The returned string
// embeds cfg.Password in cleartext, as any connection string must;
// callers MUST NOT log it.
func (cfg Config) DSN(dbName string) string {
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

// Open opens a *sql.DB against dsn using the pgx stdlib driver and
// verifies connectivity with a bounded Ping before returning, so a
// misconfigured or unreachable database fails fast with a clear error
// instead of surfacing as an opaque failure from the first real
// query. It never logs dsn (which embeds the connection password in
// cleartext, per Config.DSN's doc comment).
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: open: ping: %w", err)
	}
	return db, nil
}
