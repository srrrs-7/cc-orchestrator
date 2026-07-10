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
// exists, then apply a Command's migrations to it, then (if
// requested) provision a least-privilege runtime role.
type Service struct {
	db          migration.Database
	runner      migration.Runner
	provisioner migration.RoleProvisioner
}

// New builds a Service from its three ports. provisioner is only ever
// invoked when a Migrate call's role argument is non-nil (ISSUE-016
// R-c); a caller that never requests role provisioning may pass any
// non-nil migration.RoleProvisioner (cmd/migrator/main.go always wires
// in a real one, since constructing it is cheap and it is simply never
// called otherwise).
func New(db migration.Database, runner migration.Runner, provisioner migration.RoleProvisioner) *Service {
	return &Service{db: db, runner: runner, provisioner: provisioner}
}

// RoleRequest bundles the least-privilege runtime role Migrate should
// idempotently provision (ISSUE-016 R-c) after a successful "up"
// migration, and the password to synchronize onto it. A nil
// *RoleRequest -- cmd/migrator/main.go's default when APP_DB_USER/
// APP_DB_PASSWORD are both unset, cmd/migrator/env.go's
// backward-compatible contract -- skips provisioning entirely, so this
// migrator's pre-ISSUE-016 callers are unaffected.
type RoleRequest struct {
	Role     migration.AppRole
	Password string
}

// Migrate ensures name's database exists (creating it if it does
// not, via s.db.EnsureExists) and then runs cmd's migrations against
// it from migrationsDir (via s.runner.Run). This mirrors the
// pre-refactor run()'s two-step flow in app/migrator/main.go exactly:
// database bootstrap first, migration second, over two separate
// connections managed by the respective infra implementations.
//
// If role is non-nil and cmd is the "up" command, Migrate additionally
// provisions role.Role (creating/password-syncing it and granting it
// least privilege on its own database, via s.provisioner.EnsureAppRole)
// once the migration itself has succeeded -- ISSUE-016 R-c's "GRANT は
// goose up の後に流す" ordering requirement (plan §1.1): the
// GRANT ... ON ALL TABLES / ALTER DEFAULT PRIVILEGES statements need
// the tables goose up just created to already exist. role is silently
// ignored (no provisioning call is made) when cmd is "down" or
// "status" (migration.Command.IsUp): granting access while migrations
// are being rolled back, or merely inspected, is not this method's
// job.
func (s *Service) Migrate(ctx context.Context, name migration.DatabaseName, cmd migration.Command, migrationsDir string, role *RoleRequest) error {
	if err := s.db.EnsureExists(ctx, name); err != nil {
		return fmt.Errorf("ensure database %q exists: %w", name, err)
	}
	if err := s.runner.Run(ctx, cmd, migrationsDir); err != nil {
		return fmt.Errorf("run migration: %w", err)
	}

	if role == nil || !cmd.IsUp() {
		return nil
	}
	if err := s.provisioner.EnsureAppRole(ctx, role.Role, role.Password); err != nil {
		return fmt.Errorf("ensure app role %q: %w", role.Role, err)
	}
	return nil
}
