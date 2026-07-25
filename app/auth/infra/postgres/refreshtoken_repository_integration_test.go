package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestRefreshTokenRepository_Contract runs the behavioral contract
// shared by every refreshtoken.Repository implementation
// (SPEC-006 R4/R5/R8 / SPEC-011 R3) against a real Postgres-backed
// refreshtoken.Repository, proving single-use rotation, reuse-detection
// and TTL requirements hold for a shared, cross-instance store --
// including Rotate's transactional atomicity under concurrency.
func TestRefreshTokenRepository_Contract(t *testing.T) {
	repotest.RunRefreshTokenRepositoryContract(t, func(t *testing.T) refreshtoken.Repository {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTable(t, db, "refresh_tokens")
		return postgres.NewRefreshTokenRepository(db)
	})
}
