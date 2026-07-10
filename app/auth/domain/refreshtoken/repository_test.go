// repository_test.go is a compile-time-only proof of SPEC-010 R1 for
// the refreshtoken aggregate: refreshtoken.Repository (domain/
// refreshtoken/repository.go) must split additively into
// refreshtoken.Reader (FindByTokenHash) and refreshtoken.Writer (Save,
// Rotate, RevokeFamily), with refreshtoken.Repository remaining exactly
// their composition (interface{ Reader; Writer }). It asserts nothing
// at runtime -- if this package compiles, the three interfaces have
// exactly the shape this file's fakes assume.
package refreshtoken_test

import (
	"context"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// repoReaderOnly implements only the method refreshtoken.Reader
// declares (FindByTokenHash) and has no Save/Rotate/RevokeFamily
// method. Assigning it to a refreshtoken.Reader-typed variable is a
// compile-time proof that Reader requires nothing beyond
// FindByTokenHash.
type repoReaderOnly struct{}

func (repoReaderOnly) FindByTokenHash(ctx context.Context, hash refreshtoken.TokenHash) (*refreshtoken.RefreshToken, error) {
	return nil, refreshtoken.ErrNotFound
}

// repoWriterOnly implements only refreshtoken.Writer's methods (Save,
// Rotate, RevokeFamily) and has no FindByTokenHash method. Assigning it
// to a refreshtoken.Writer-typed variable is a compile-time proof that
// Writer requires nothing beyond those three.
type repoWriterOnly struct{}

func (repoWriterOnly) Save(ctx context.Context, rt *refreshtoken.RefreshToken) error {
	return nil
}

func (repoWriterOnly) Rotate(ctx context.Context, oldHash refreshtoken.TokenHash, newRT *refreshtoken.RefreshToken) error {
	return nil
}

func (repoWriterOnly) RevokeFamily(ctx context.Context, familyID refreshtoken.FamilyID) error {
	return nil
}

// repoReaderWriter embeds both narrow fakes above, promoting their
// methods, so it satisfies the full set FindByTokenHash/Save/Rotate/
// RevokeFamily.
type repoReaderWriter struct {
	repoReaderOnly
	repoWriterOnly
}

// TestRepository_ReaderWriterSplit is SPEC-010 R1's compile-time
// contract for the refreshtoken aggregate: Reader and Writer must be
// exactly as narrow as documented, and their composition must still
// satisfy the pre-existing Repository interface every current consumer
// (service.AuthorizationService, repotest, infra/memory,
// infra/postgres) depends on.
func TestRepository_ReaderWriterSplit(t *testing.T) {
	var _ refreshtoken.Reader = repoReaderOnly{}
	var _ refreshtoken.Writer = repoWriterOnly{}
	var _ refreshtoken.Repository = repoReaderWriter{}
}
