//go:build integration

package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestUserRepository_Contract runs the same behavioral contract as
// infra/memory (infra/memory/user_repository_contract_test.go)
// against a real Postgres-backed user.Repository, proving R2 for the
// user aggregate.
//
// Seeding goes through postgres.SeedUser (planned; see
// docs/plans/SPEC-005-plan.md §2.2 "UpsertUser :exec" /
// infra/postgres/seed.go), since user.Repository itself is read-only
// (FindByID / FindByUsername only).
func TestUserRepository_Contract(t *testing.T) {
	repotest.RunUserRepositoryContract(t, func(t *testing.T, seed ...*user.User) user.Repository {
		db := openTestDB(t)
		truncateTable(t, db, "users")
		for _, u := range seed {
			if err := postgres.SeedUser(t.Context(), db, u); err != nil {
				t.Fatalf("SeedUser(%v) unexpected error: %v", u.ID(), err)
			}
		}
		return postgres.NewUserRepository(db)
	})
}
