package memory_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestRefreshTokenRepository_Contract runs the behavioral contract
// shared by every refreshtoken.Repository implementation (SPEC-006
// R4/R5/R8) against infra/memory: Save/FindByTokenHash round-trip
// (including a consumed-but-unexpired row), the atomic single-use +
// rotation mechanism (Rotate, including its ErrReused vs ErrNotFound
// precedence and behavior under concurrency), family-wide revocation
// (RevokeFamily), and TTL-based expiry (including lazy eviction). The
// identical suite (repotest.RunRefreshTokenRepositoryContract) is
// exercised against infra/postgres by
// infra/postgres/refreshtoken_repository_integration_test.go (build
// tag "integration"). See refreshtoken_repository_test.go in this
// package for additional infra/memory-specific tests (e.g. clone
// independence) that are not part of the aggregate's shared Repository
// contract.
func TestRefreshTokenRepository_Contract(t *testing.T) {
	repotest.RunRefreshTokenRepositoryContract(t, func(t *testing.T) refreshtoken.Repository {
		t.Helper()
		return memory.NewRefreshTokenRepository()
	})
}
