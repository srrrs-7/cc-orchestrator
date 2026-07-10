// Command migrator is SPEC-005's shared migration runner for app/api
// and app/auth (2026-07-09 refactor, plan §RF.1.2). Given -target
// api|auth it:
//
//  1. Ensures that stack's dedicated Postgres database exists,
//     creating it if not (infra/postgres.EnsureExister, the
//     migration.Database port's implementation) -- api and auth each
//     get their own database on the same Postgres instance (plan
//     §RF.1.1), replacing the prior single-database/separate-schema
//     design.
//  2. Applies that stack's db/migrations against it via goose, run as
//     a library (infra/goose.Runner, the migration.Runner port's
//     implementation, backed by github.com/pressly/goose/v3) rather
//     than the `go run` CLI app/{api,auth}/Makefile's migrate-create
//     still uses, so app/api and app/auth's own go.mod never need to
//     require goose directly -- their only new runtime dependency
//     stays pgx (plan §RF.1.3).
//  3. If APP_DB_USER/APP_DB_PASSWORD are set and -command is up, idempotently
//     provisions that stack's least-privilege runtime role (infra/
//     postgres.RoleEnsurer, the migration.RoleProvisioner port's
//     implementation): CONNECT to its own database only, DML on
//     existing and future tables/sequences in schema public, never DDL
//     (ISSUE-016 R-c). Both variables unset -- this migrator's
//     pre-ISSUE-016 default -- skips this step entirely and changes
//     nothing about steps 1-2.
//
// This file is app/migrator's composition root (mirrors
// app/api/cmd/api/main.go and app/auth/cmd/authz/main.go): it parses
// CLI flags into the migration domain's value objects (domain/
// migration), reads and validates the process environment (env.go,
// the only file in this module that calls os.Getenv), wires the
// infra/postgres and infra/goose port implementations into
// service.Service, and runs it. It holds no migration logic itself.
//
// Usage:
//
//	migrator -target api|auth [-command up|down|status] [-migrations-dir <path>]
//
// Connection settings are read from the discrete DB_* environment
// variables documented in env.go's Env/NewEnv/validate: DB_HOST/
// DB_PORT/DB_USER/DB_PASSWORD/DB_SSLMODE/DB_NAME/DB_MAINTENANCE_NAME,
// plus the optional APP_DB_USER/APP_DB_PASSWORD pair (env.go's
// appRole) naming the least-privilege runtime role to provision.
// Neither the assembled DSN nor DB_PASSWORD/APP_DB_PASSWORD is ever
// logged; a failure exits non-zero (so an ECS init container's
// dependsOn: SUCCESS gate, or a CI step, observes it as a failure)
// without leaking credentials in its message.
//
// The goose run itself (as opposed to the initial connectivity checks,
// bounded by infra/postgres.Open's own pingTimeout) is bounded by
// env.go's defaultMigrationTimeout (5 minutes), overridable via the
// MIGRATOR_TIMEOUT environment variable (a Go time.Duration string,
// e.g. "10m"). This keeps a hung migration from blocking an ECS init
// container's dependsOn: SUCCESS gate forever.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
	"github.com/srrrs-7/cc-orchestrator/app/migrator/infra/goose"
	"github.com/srrrs-7/cc-orchestrator/app/migrator/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/migrator/service"
)

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

	e := NewEnv()
	cfg, dbName, err := e.validate(target)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}

	appRole, appPassword, roleRequested, err := e.appRole(dbName)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}

	svc := service.New(
		postgres.NewEnsureExister(cfg),
		goose.NewRunner(cfg, dbName, e.MigratorTimeout),
		postgres.NewRoleEnsurer(cfg),
	)

	var roleReq *service.RoleRequest
	if roleRequested {
		roleReq = &service.RoleRequest{Role: appRole, Password: appPassword}
	}

	if err := svc.Migrate(ctx, dbName, command, migrationsDir, roleReq); err != nil {
		return fmt.Errorf("migrator: %w", err)
	}

	slog.Info("migrator: done",
		"target", target, "database", dbName, "command", command, "migrations_dir", migrationsDir,
		"role_provisioned", roleRequested && command.IsUp(),
	)
	return nil
}

// parseFlags parses and validates this binary's CLI contract
// (-target, -command, -migrations-dir), applying -migrations-dir's
// default (target.DefaultMigrationsDir(), "/migrations/<target>",
// matching the Dockerfile's COPY layout: COPY app/api/db/migrations
// /migrations/api and the auth equivalent) when the flag is not
// explicitly set. -target and -command are validated by the migration
// domain (migration.ParseTarget / migration.ParseCommand); this
// function only adds the CLI flag name to the resulting error so it
// is actionable from the command line.
func parseFlags(args []string) (target migration.Target, command migration.Command, migrationsDir string, err error) {
	fs := flag.NewFlagSet("migrator", flag.ContinueOnError)
	targetFlag := fs.String("target", "", "target stack to migrate: api or auth (required)")
	commandFlag := fs.String("command", "up", "goose command to run: up, down, or status")
	migrationsDirFlag := fs.String("migrations-dir", "", `migrations directory (default "/migrations/<target>")`)
	if err := fs.Parse(args); err != nil {
		return migration.Target{}, migration.Command{}, "", err
	}

	target, err = migration.ParseTarget(*targetFlag)
	if err != nil {
		return migration.Target{}, migration.Command{}, "", fmt.Errorf("-target: %w", err)
	}
	command, err = migration.ParseCommand(*commandFlag)
	if err != nil {
		return migration.Target{}, migration.Command{}, "", fmt.Errorf("-command: %w", err)
	}

	dir := *migrationsDirFlag
	if dir == "" {
		dir = target.DefaultMigrationsDir()
	}

	return target, command, dir, nil
}
