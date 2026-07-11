package postgres_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/repotest"
)

// TestAuthCodeRepository_Contract runs the behavioral contract shared
// by every authcode.Repository implementation (SPEC-005 R2 / SPEC-011 R3)
// against a real Postgres-backed authcode.Repository. Unlike client/user,
// authcode.Repository.Save is part of the domain port, so no separate
// seed helper is needed.
func TestAuthCodeRepository_Contract(t *testing.T) {
	repotest.RunAuthCodeRepositoryContract(t, func(t *testing.T) authcode.Repository {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTable(t, db, "authorization_codes")
		return postgres.NewAuthCodeRepository(db)
	})
}
