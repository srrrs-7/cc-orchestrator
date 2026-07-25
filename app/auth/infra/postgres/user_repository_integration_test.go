package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestUserRepository_Contract runs the behavioral contract shared by
// every user.Repository implementation (SPEC-005 R2 / SPEC-011 R3)
// against a real Postgres-backed user.Repository.
//
// Seeding goes through testsupport.SeedUser (wrapping
// postgres.SeedUser's upsert), since user.Repository itself is
// read-only (FindByID / FindByUsername only).
func TestUserRepository_Contract(t *testing.T) {
	repotest.RunUserRepositoryContract(t, func(t *testing.T, seed ...*user.User) user.Repository {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTable(t, db, "users")
		for _, u := range seed {
			testsupport.SeedUser(t, db, u)
		}
		return postgres.NewUserRepository(db)
	})
}
