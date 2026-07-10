//go:build integration

package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestRefreshTokenRepository_Contract runs the same behavioral
// contract as infra/memory
// (infra/memory/refreshtoken_repository_contract_test.go) against a
// real Postgres-backed refreshtoken.Repository, proving SPEC-006's
// R4/R5/R8 single-use rotation, reuse-detection and TTL requirements
// hold for a shared, cross-instance store -- including Rotate's
// transactional atomicity under concurrency (see
// RefreshTokenRepository.Rotate's doc comment).
func TestRefreshTokenRepository_Contract(t *testing.T) {
	repotest.RunRefreshTokenRepositoryContract(t, func(t *testing.T) refreshtoken.Repository {
		db := openTestDB(t)
		truncateTable(t, db, "refresh_tokens")
		return postgres.NewRefreshTokenRepository(db)
	})
}
