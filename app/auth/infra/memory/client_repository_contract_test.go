package memory_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestClientRepository_Contract runs the behavioral contract shared
// by every client.Repository implementation (SPEC-005 R2) against
// infra/memory. The identical suite (repotest.RunClientRepositoryContract)
// is exercised against infra/postgres by
// infra/postgres/client_repository_integration_test.go (build tag
// "integration").
func TestClientRepository_Contract(t *testing.T) {
	repotest.RunClientRepositoryContract(t, func(t *testing.T, seed ...*client.Client) client.Repository {
		t.Helper()
		repo := memory.NewClientRepository()
		for _, c := range seed {
			repo.Seed(c)
		}
		return repo
	})
}
