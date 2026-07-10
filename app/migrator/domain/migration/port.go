package migration

import "context"

// Database is the persistence-bootstrap boundary service.Service
// relies on: make sure a given DatabaseName exists before anything is
// migrated against it. It is defined in the domain layer (dependency
// inversion, same pattern as app/api/domain/task.Repository): the
// domain declares what it needs, and infra/postgres.EnsureExister
// provides the concrete implementation.
type Database interface {
	// EnsureExists creates the database named name if it does not
	// already exist. It must be idempotent: a database that already
	// exists (including one created concurrently by another migrator
	// invocation racing this one) is success, not an error.
	EnsureExists(ctx context.Context, name DatabaseName) error
}

// Runner is the migration-execution boundary service.Service relies
// on: apply cmd's migrations from migrationsDir against the target
// database this Runner was built for. infra/goose.Runner provides the
// concrete implementation (goose, run as a library).
type Runner interface {
	Run(ctx context.Context, cmd Command, migrationsDir string) error
}
