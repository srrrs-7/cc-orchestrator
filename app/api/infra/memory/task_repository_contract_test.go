package memory_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/repotest"
)

// TestTaskRepository_Contract runs the behavioral contract shared by
// every task.Repository implementation (SPEC-005 R1) against
// infra/memory. The identical suite (repotest.RunTaskRepositoryContract)
// is exercised against infra/postgres by
// infra/postgres/task_repository_integration_test.go (build tag
// "integration"), so a passing run here is half of the R1 acceptance
// criteria: "infra/postgres behaves identically to infra/memory".
func TestTaskRepository_Contract(t *testing.T) {
	repotest.RunTaskRepositoryContract(t, func(t *testing.T) task.Repository {
		t.Helper()
		return memory.NewTaskRepository()
	})
}
