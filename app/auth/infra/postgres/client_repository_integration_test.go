//go:build integration

package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestClientRepository_Contract runs the behavioral contract shared by
// every client.Repository implementation (SPEC-005 R2 / SPEC-011 R3)
// against a real Postgres-backed client.Repository.
//
// Seeding goes through testsupport.SeedClient (wrapping
// postgres.SeedClient's upsert), since client.Repository itself is
// read-only (FindByID only).
func TestClientRepository_Contract(t *testing.T) {
	repotest.RunClientRepositoryContract(t, func(t *testing.T, seed ...*client.Client) client.Repository {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTable(t, db, "clients")
		for _, c := range seed {
			testsupport.SeedClient(t, db, c)
		}
		return postgres.NewClientRepository(db)
	})
}
