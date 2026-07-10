// Package service is app/migrator's application layer: it
// orchestrates the migration domain's two ports (migration.Database /
// migration.Runner) without knowing which concrete infra
// implementation (infra/postgres, infra/goose) backs them --
// cmd/migrator/main.go wires those in. Compare app/api/service, which
// plays the same coordinating role between app/api/domain/task and
// app/api/infra.
package service

import (
	"context"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
)

// Service runs a single migration: ensure the target database
// exists, then apply a Command's migrations to it.
type Service struct {
	db     migration.Database
	runner migration.Runner
}

// New builds a Service from its two ports.
func New(db migration.Database, runner migration.Runner) *Service {
	return &Service{db: db, runner: runner}
}

// Migrate ensures name's database exists (creating it if it does
// not, via s.db.EnsureExists) and then runs cmd's migrations against
// it from migrationsDir (via s.runner.Run). This mirrors the
// pre-refactor run()'s two-step flow in app/migrator/main.go exactly:
// database bootstrap first, migration second, over two separate
// connections managed by the respective infra implementations.
func (s *Service) Migrate(ctx context.Context, name migration.DatabaseName, cmd migration.Command, migrationsDir string) error {
	if err := s.db.EnsureExists(ctx, name); err != nil {
		return fmt.Errorf("ensure database %q exists: %w", name, err)
	}
	if err := s.runner.Run(ctx, cmd, migrationsDir); err != nil {
		return fmt.Errorf("run migration: %w", err)
	}
	return nil
}
