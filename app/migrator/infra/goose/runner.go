// Package goose is app/migrator's implementation of the
// migration.Runner port: applying a migration.Command's migrations
// via github.com/pressly/goose/v3, run as a *library* dependency
// (require in go.mod) rather than the `go run pkg@version` CLI
// app/{api,auth}/Makefile's migrate-create target uses -- see this
// module's go.mod doc comment and .claude/rules/db.md "goose の閉じ込め".
package goose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gooselib "github.com/pressly/goose/v3"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
	"github.com/srrrs-7/cc-orchestrator/app/migrator/infra/postgres"
)

// Runner implements migration.Runner against a real Postgres
// database: the target stack's own database (as opposed to the
// maintenance database infra/postgres.EnsureExister connects to).
// Run opens its own connection to cfg/dbName, applies cmd via goose,
// and closes that connection before returning -- a fresh connection
// per Run call, matching the pre-refactor main.go's run(), which
// opened the target connection once per process invocation.
type Runner struct {
	cfg     postgres.Config
	dbName  migration.DatabaseName
	timeout time.Duration
}

// NewRunner builds a Runner. timeout bounds the goose run itself (as
// opposed to the connection's initial ping, bounded by
// infra/postgres.Open's own fixed pingTimeout) -- see
// cmd/migrator/env.go's MIGRATOR_TIMEOUT doc comment for where timeout
// comes from.
func NewRunner(cfg postgres.Config, dbName migration.DatabaseName, timeout time.Duration) *Runner {
	return &Runner{cfg: cfg, dbName: dbName, timeout: timeout}
}

// Run implements migration.Runner: it connects to r.dbName, sets
// goose's dialect, and runs cmd (up/down/status) against
// migrationsDir, bounded by r.timeout so a hung migration (e.g.
// blocked waiting on a lock another session holds on the target
// database) fails fast instead of running forever -- this matters
// because an ECS init container's dependsOn: SUCCESS gate (or a CI
// step) would otherwise block indefinitely rather than observing a
// fast, actionable failure. r.timeout is a separate, larger budget
// than the connection's own initial-ping timeout
// (infra/postgres.Open's fixed pingTimeout).
func (r *Runner) Run(ctx context.Context, cmd migration.Command, migrationsDir string) error {
	db, err := postgres.Open(ctx, r.cfg.DSN(r.dbName.String()))
	if err != nil {
		return fmt.Errorf("open target database %q: %w", r.dbName, err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("migrator: close target database", "error", closeErr)
		}
	}()

	if err := gooselib.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if err := gooselib.RunContext(runCtx, cmd.String(), db, migrationsDir); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("run goose %s (dir=%s) exceeded timeout %s: %w", cmd, migrationsDir, r.timeout, err)
		}
		return fmt.Errorf("run goose %s (dir=%s): %w", cmd, migrationsDir, err)
	}
	return nil
}
