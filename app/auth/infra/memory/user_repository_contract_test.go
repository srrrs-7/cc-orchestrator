package memory_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestUserRepository_Contract runs the behavioral contract shared by
// every user.Repository implementation (SPEC-005 R2) against
// infra/memory. The identical suite (repotest.RunUserRepositoryContract)
// is exercised against infra/postgres by
// infra/postgres/user_repository_integration_test.go (build tag
// "integration").
func TestUserRepository_Contract(t *testing.T) {
	repotest.RunUserRepositoryContract(t, func(t *testing.T, seed ...*user.User) user.Repository {
		t.Helper()
		repo := memory.NewUserRepository()
		for _, u := range seed {
			repo.Seed(u)
		}
		return repo
	})
}
