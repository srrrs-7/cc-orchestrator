//go:build integration

package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestAuthCodeRepository_Contract runs the same behavioral contract
// as infra/memory (infra/memory/authcode_repository_contract_test.go)
// against a real Postgres-backed authcode.Repository, proving R2's
// single-use + TTL requirements hold for a shared, cross-instance
// store (the concrete problem statement in SPEC-005 §1: "同じコードが別
// インスタンスで再利用され得る"). Unlike client/user,
// authcode.Repository.Save is part of the domain port, so no separate
// seed helper is needed here.
func TestAuthCodeRepository_Contract(t *testing.T) {
	repotest.RunAuthCodeRepositoryContract(t, func(t *testing.T) authcode.Repository {
		db := openTestDB(t)
		truncateTable(t, db, "authorization_codes")
		return postgres.NewAuthCodeRepository(db)
	})
}
