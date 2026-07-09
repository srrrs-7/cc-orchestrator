//go:build integration

package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestClientRepository_Contract runs the same behavioral contract as
// infra/memory (infra/memory/client_repository_contract_test.go)
// against a real Postgres-backed client.Repository, proving R2 for
// the client aggregate.
//
// Seeding goes through postgres.SeedClient (planned; see
// docs/plans/SPEC-005-plan.md §2.2 "UpsertClient :exec" /
// infra/postgres/seed.go), since client.Repository itself is
// read-only (FindByID only).
func TestClientRepository_Contract(t *testing.T) {
	repotest.RunClientRepositoryContract(t, func(t *testing.T, seed ...*client.Client) client.Repository {
		db := openTestDB(t)
		truncateTable(t, db, "clients")
		for _, c := range seed {
			if err := postgres.SeedClient(t.Context(), db, c); err != nil {
				t.Fatalf("SeedClient(%v) unexpected error: %v", c.ID(), err)
			}
		}
		return postgres.NewClientRepository(db)
	})
}
