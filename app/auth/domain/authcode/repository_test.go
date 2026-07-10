// repository_test.go is a compile-time-only proof of SPEC-010 R1 for
// the authcode aggregate: authcode.Repository (domain/authcode/
// repository.go) must split additively into authcode.Reader
// (FindByCode) and authcode.Writer (Save, Consume), with
// authcode.Repository remaining exactly their composition
// (interface{ Reader; Writer }). It asserts nothing at runtime -- if
// this package compiles, the three interfaces have exactly the shape
// this file's fakes assume.
package authcode_test

import (
	"context"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

// repoReaderOnly implements only the method authcode.Reader declares
// (FindByCode) and has no Save/Consume method. Assigning it to an
// authcode.Reader-typed variable is a compile-time proof that Reader
// requires nothing beyond FindByCode.
type repoReaderOnly struct{}

func (repoReaderOnly) FindByCode(ctx context.Context, code authcode.Code) (*authcode.AuthorizationCode, error) {
	return nil, authcode.ErrNotFound
}

// repoWriterOnly implements only authcode.Writer's methods (Save,
// Consume) and has no FindByCode method. Assigning it to an
// authcode.Writer-typed variable is a compile-time proof that Writer
// requires nothing beyond Save/Consume.
type repoWriterOnly struct{}

func (repoWriterOnly) Save(ctx context.Context, ac *authcode.AuthorizationCode) error { return nil }
func (repoWriterOnly) Consume(ctx context.Context, code authcode.Code) error          { return nil }

// repoReaderWriter embeds both narrow fakes above, promoting their
// methods, so it satisfies the full set FindByCode/Save/Consume.
type repoReaderWriter struct {
	repoReaderOnly
	repoWriterOnly
}

// TestRepository_ReaderWriterSplit is SPEC-010 R1's compile-time
// contract for the authcode aggregate: Reader and Writer must be
// exactly as narrow as documented, and their composition must still
// satisfy the pre-existing Repository interface every current consumer
// (service.AuthorizationService, repotest, infra/memory,
// infra/postgres) depends on.
func TestRepository_ReaderWriterSplit(t *testing.T) {
	var _ authcode.Reader = repoReaderOnly{}
	var _ authcode.Writer = repoWriterOnly{}
	var _ authcode.Repository = repoReaderWriter{}
}
