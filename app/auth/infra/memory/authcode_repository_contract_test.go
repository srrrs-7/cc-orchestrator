package memory_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestAuthCodeRepository_Contract runs the behavioral contract shared
// by every authcode.Repository implementation (SPEC-005 R2) against
// infra/memory: Save/FindByCode round-trip, single-use Consume, and
// TTL-based expiry (including lazy eviction and atomicity under
// concurrent Consume). The identical suite
// (repotest.RunAuthCodeRepositoryContract) is exercised against
// infra/postgres by
// infra/postgres/authcode_repository_integration_test.go (build tag
// "integration"). See authcode_repository_test.go in this package for
// additional infra/memory-specific tests (e.g. re-Save after Consume,
// clone independence) that are not part of the aggregate's shared
// Repository contract.
func TestAuthCodeRepository_Contract(t *testing.T) {
	repotest.RunAuthCodeRepositoryContract(t, func(t *testing.T) authcode.Repository {
		t.Helper()
		return memory.NewAuthCodeRepository()
	})
}
