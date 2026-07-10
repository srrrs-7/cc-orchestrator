// Command migrator is SPEC-005's shared migration runner for app/api
// and app/auth (2026-07-09 refactor, plan §RF.1.2). Given -target
// api|auth it:
//
//  1. Ensures that stack's dedicated Postgres database exists,
//     creating it if not (ensureDatabase in database.go) -- api and
//     auth each get their own database on the same Postgres instance
//     (plan §RF.1.1), replacing the prior single-database/
//     separate-schema design.
//  2. Applies that stack's db/migrations against it via goose, run as
//     a library (github.com/pressly/goose/v3) rather than the `go run`
//     CLI app/{api,auth}/Makefile's migrate-create still uses, so
//     app/api and app/auth's own go.mod never need to require goose
//     directly -- their only new runtime dependency stays pgx (plan
//     §RF.1.3).
//
// Usage:
//
//	migrator -target api|auth [-command up|down|status] [-migrations-dir <path>]
//
// Connection settings are read from the discrete DB_* environment
// variables documented in config.go's Config/configFromEnv: DB_HOST/
// DB_PORT/DB_USER/DB_PASSWORD/DB_SSLMODE/DB_NAME/DB_MAINTENANCE_NAME.
// Neither the assembled DSN nor DB_PASSWORD is ever logged; a failure
// exits non-zero (so an ECS init container's dependsOn: SUCCESS gate,
// or a CI step, observes it as a failure) without leaking credentials
// in its message.
//
// The goose run itself (as opposed to the initial connectivity checks,
// bounded by pingTimeout) is bounded by defaultMigrationTimeout (5
// minutes), overridable via the MIGRATOR_TIMEOUT environment variable
// (a Go time.Duration string, e.g. "10m"); see migrationTimeout. This
// keeps a hung migration from blocking an ECS init container's
// dependsOn: SUCCESS gate forever.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver name
)

// pingTimeout bounds how long openWithPing waits for the initial
// connectivity check on each of the two connections run() opens
// (maintenance, then target), matching
// app/{api,auth}/infra/postgres/db.go's Open exactly.
const pingTimeout = 5 * time.Second

// defaultMigrationTimeout bounds how long the goose migration run
// itself (goose.RunContext in run()) may take. Without this, run()'s
// ctx only cancels on SIGINT/SIGTERM (signal.NotifyContext), so a
// migration that hangs -- e.g. blocked waiting on a lock another
// session holds on the target database -- would run forever instead
// of failing. That matters here specifically because an ECS init
// container's dependsOn: SUCCESS gate (or a CI step) would then block
// indefinitely rather than observing a fast, actionable failure.
// pingTimeout above bounds only the initial connectivity check, not
// the migration run that follows it, so this is a separate, larger
// budget. Overridable via MIGRATOR_TIMEOUT (a Go time.Duration string,
// e.g. "10m") for environments where a few minutes is not enough
// (large backlogs of pending migrations, contended locks under load);
// see migrationTimeout.
const defaultMigrationTimeout = 5 * time.Minute

// validTargets is the closed set of -target values this binary
// accepts (SPEC-005 R5: "-target api|auth"). Both the default
// migrations directory ("/migrations/<target>", matching this image's
// Dockerfile COPY layout) and the default database name (plan
// §RF.1.1: "api"/"auth") derive from target, so an unrecognized value
// is rejected outright rather than silently producing an empty or
// unexpected path/name.
var validTargets = map[string]bool{"api": true, "auth": true}

// validCommands is the closed set of -command values this binary
// forwards to goose.RunContext (SPEC-005 R5: "-command up|down|status").
var validCommands = map[string]bool{"up": true, "down": true, "status": true}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("migrator: fatal error", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	target, command, migrationsDir, err := parseFlags(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := configFromEnv(target)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}

	if err := ensureTargetDatabase(ctx, cfg); err != nil {
		return fmt.Errorf("migrator: %w", err)
	}

	db, err := openWithPing(ctx, cfg.dsn(cfg.Name))
	if err != nil {
		return fmt.Errorf("migrator: open target database %q: %w", cfg.Name, err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("migrator: close target database", "error", closeErr)
		}
	}()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrator: set goose dialect: %w", err)
	}

	timeout := migrationTimeout()
	runCtx, cancelRun := context.WithTimeout(ctx, timeout)
	defer cancelRun()
	if err := goose.RunContext(runCtx, command, db, migrationsDir); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("migrator: run goose %s (dir=%s) exceeded timeout %s: %w", command, migrationsDir, timeout, err)
		}
		return fmt.Errorf("migrator: run goose %s (dir=%s): %w", command, migrationsDir, err)
	}

	slog.Info("migrator: done", "target", target, "database", cfg.Name, "command", command, "migrations_dir", migrationsDir)
	return nil
}

// parseFlags parses and validates this binary's CLI contract
// (-target, -command, -migrations-dir), applying -migrations-dir's
// default ("/migrations/<target>", matching the Dockerfile's COPY
// layout: COPY app/api/db/migrations /migrations/api and the auth
// equivalent) when the flag is not explicitly set.
func parseFlags(args []string) (target, command, migrationsDir string, err error) {
	fs := flag.NewFlagSet("migrator", flag.ContinueOnError)
	targetFlag := fs.String("target", "", "target stack to migrate: api or auth (required)")
	commandFlag := fs.String("command", "up", "goose command to run: up, down, or status")
	migrationsDirFlag := fs.String("migrations-dir", "", `migrations directory (default "/migrations/<target>")`)
	if err := fs.Parse(args); err != nil {
		return "", "", "", err
	}

	if !validTargets[*targetFlag] {
		return "", "", "", fmt.Errorf("migrator: -target must be one of api, auth (got %q)", *targetFlag)
	}
	if !validCommands[*commandFlag] {
		return "", "", "", fmt.Errorf("migrator: -command must be one of up, down, status (got %q)", *commandFlag)
	}

	dir := *migrationsDirFlag
	if dir == "" {
		dir = "/migrations/" + *targetFlag
	}

	return *targetFlag, *commandFlag, dir, nil
}

// migrationTimeout resolves the goose run's timeout budget: the
// MIGRATOR_TIMEOUT environment variable if set and parsable as a Go
// time.Duration (e.g. "10m", "90s"), otherwise
// defaultMigrationTimeout. A set-but-unparsable value is treated the
// same as unset (falls back to the default) but is logged as a
// warning so a typo'd override is visible instead of silently
// behaving as if it were never set.
func migrationTimeout() time.Duration {
	raw := os.Getenv("MIGRATOR_TIMEOUT")
	if raw == "" {
		return defaultMigrationTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		slog.Warn("migrator: invalid MIGRATOR_TIMEOUT, using default", "value", raw, "default", defaultMigrationTimeout, "error", err)
		return defaultMigrationTimeout
	}
	return d
}

// ensureTargetDatabase opens a short-lived connection to cfg's
// maintenance database (default "postgres") and delegates to
// ensureDatabase (database.go) to create cfg.Name if it does not
// already exist. This connection is closed before run() opens its
// second, longer-lived connection to cfg.Name itself: CREATE DATABASE
// must run outside any transaction against a database other than the
// one being created (plan §RF.1.2), and there is no reason to hold
// this maintenance connection open once that step is done.
func ensureTargetDatabase(ctx context.Context, cfg Config) error {
	maintenanceDB, err := openWithPing(ctx, cfg.dsn(cfg.MaintenanceName))
	if err != nil {
		return fmt.Errorf("open maintenance database %q: %w", cfg.MaintenanceName, err)
	}
	defer func() {
		if closeErr := maintenanceDB.Close(); closeErr != nil {
			slog.Error("migrator: close maintenance database", "error", closeErr)
		}
	}()

	return ensureDatabase(ctx, maintenanceDB, cfg.Name)
}

// openWithPing opens a *sql.DB against dsn using the pgx stdlib driver
// and verifies connectivity with a bounded Ping before returning, so a
// misconfigured or unreachable database fails fast with a clear error
// instead of surfacing as an opaque failure from the first real query.
// It never logs dsn (which embeds the connection password in
// cleartext, per Config.dsn's doc comment).
func openWithPing(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}
