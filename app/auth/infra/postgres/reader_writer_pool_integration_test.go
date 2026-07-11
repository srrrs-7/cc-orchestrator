//go:build integration

// reader_writer_pool_integration_test.go is SPEC-010's integration
// suite for app/auth's infra/postgres writer/reader pool split
// (docs/plans/SPEC-010-plan.md フェーズ1): postgres.OpenPair's pool-
// sharing/opening decision, plus proof that authcode.Repository's
// single-use-sensitive reads stay fixed to the writer pool regardless
// of the (would-be-replica) reader pool's lifecycle
// (SPEC-010-plan.md "auth の correctness-critical read の配置").
//
// As of the TDD "red" phase, infra/postgres does not have OpenPair
// yet: this file is written against its *planned* signature
// (SPEC-010-plan.md "固定された対象シグネチャ") and therefore
// intentionally fails to compile with -tags=integration until impl-db
// lands infra/postgres/db.go's OpenPair. That is expected and does not
// affect the default (untagged) build/vet/test, which never parses
// this file.
//
// Per the plan's "別ホスト reader の再現" note, both suites below avoid
// requiring a second live database: OpenPair's sharing test uses a
// single reachable Config twice; the authcode fixed-to-writer test
// opens two independent *sql.DB pools (via the existing openTestDB
// helper, called twice) against the same reachable Postgres instance
// and proves the authcode repository -- deliberately constructed with
// only the writer pool, per this Spec's wiring decision -- is
// unaffected by the reader pool's lifecycle.
package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
)

// TestOpenPair_SharesPoolWhenConfigEqual covers SPEC-010 R3 and the
// non-functional "二重に開かない" requirement: OpenPair called with an
// identical writer/reader Config must not open a second *sql.DB -- the
// returned reader pointer must be the exact same *sql.DB as writer.
func TestOpenPair_SharesPoolWhenConfigEqual(t *testing.T) {
	if os.Getenv("DB_HOST") == "" {
		t.Skip("DB_HOST not set; skipping infra/postgres integration test (see docs/plans/SPEC-005-plan.md §0)")
	}
	cfg := testsupport.TestConfig()
	ctx := context.Background()

	writer, reader, closeFn, err := postgres.OpenPair(ctx, cfg, cfg)
	if err != nil {
		t.Fatalf("OpenPair() with equal writer/reader Config unexpected error: %v", err)
	}
	t.Cleanup(func() {
		if err := closeFn(); err != nil {
			t.Errorf("closeFn() unexpected error: %v", err)
		}
	})

	if writer == nil || reader == nil {
		t.Fatalf("OpenPair() returned a nil pool: writer=%v reader=%v", writer, reader)
	}
	if writer != reader {
		t.Error("OpenPair() with equal writer/reader Config must share a single *sql.DB pool (writer == reader), got distinct pointers")
	}
	if err := writer.PingContext(ctx); err != nil {
		t.Errorf("shared pool PingContext() unexpected error: %v", err)
	}
}

// TestOpenPair_DifferentReaderConfig_FailsWithoutLeakingWriter covers
// SPEC-010's R-5 risk ("別ホストで reader open が失敗したら writer を
// close して error を返す(リーク防止)"): a readerCfg that differs from
// writerCfg (so OpenPair must attempt to open a second, independent
// pool rather than sharing writer's) but names an unreachable host must
// surface as an error, with no usable *sql.DB handles returned.
func TestOpenPair_DifferentReaderConfig_FailsWithoutLeakingWriter(t *testing.T) {
	if os.Getenv("DB_HOST") == "" {
		t.Skip("DB_HOST not set; skipping infra/postgres integration test (see docs/plans/SPEC-005-plan.md §0)")
	}
	writerCfg := testsupport.TestConfig()
	readerCfg := writerCfg
	// "invalid" is an IANA-reserved TLD (RFC 2606) guaranteed never to
	// resolve, so this failure is deterministic and independent of the
	// surrounding network/CI topology.
	readerCfg.Host = "unreachable-reader.invalid"

	ctx := context.Background()
	writer, reader, closeFn, err := postgres.OpenPair(ctx, writerCfg, readerCfg)
	if err == nil {
		t.Fatalf("OpenPair() with an unreachable reader host succeeded, want an error (writer=%v reader=%v)", writer, reader)
	}
	if writer != nil || reader != nil {
		t.Errorf("OpenPair() on error must not return usable pools, got writer=%v reader=%v", writer, reader)
	}
	if closeFn != nil {
		if cerr := closeFn(); cerr != nil {
			t.Errorf("closeFn() returned alongside an OpenPair() error must still be safe to call, got: %v", cerr)
		}
	}
}

// pkceChallenge independently computes the RFC 7636 S256 transformation
// (mirrors app/auth/route/helpers_test.go's own copy; duplicated here
// rather than exported since it is a one-line, test-only convenience).
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// newPoolRoutingAuthCode builds a minimal, valid *authcode.AuthorizationCode
// for the writer-pool-fixed test below (mirrors
// infra/repotest.newTestAuthCode's fixture shape).
func newPoolRoutingAuthCode(t *testing.T) *authcode.AuthorizationCode {
	t.Helper()

	scope, err := authcode.ParseScope("openid")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}
	verifier := "pool-routing-test-code-verifier-4321"
	challenge, err := authcode.NewCodeChallenge(pkceChallenge(verifier), authcode.CodeChallengeMethodS256)
	if err != nil {
		t.Fatalf("setup NewCodeChallenge() unexpected error: %v", err)
	}

	ac, err := authcode.New(
		authcode.NewClientID("pool-routing-client"),
		authcode.NewUserID("pool-routing-user"),
		authcode.NewRedirectURI("http://localhost/callback"),
		scope,
		authcode.NewNonce(""),
		challenge,
	)
	if err != nil {
		t.Fatalf("setup New() unexpected error: %v", err)
	}
	return ac
}

// TestAuthCodeRepository_FixedToWriterPool_UnaffectedByReaderPoolClose
// is SPEC-010's auth-side pool-routing proof (plan §"auth の
// correctness-critical read の配置": authcode's FindByCode/Consume must
// stay bound to the writer pool, never the reader pool, so a single-use
// code's read-after-write is never exposed to replica lag). It uses the
// same "close で可視化" technique as the api-side test: two independent
// *sql.DB pools opened against the same reachable database stand in
// for a writer pool and a (would-be-replica) reader pool.
// postgres.NewAuthCodeRepository's constructor is unchanged by SPEC-010
// (it still takes a single *sql.DB) -- the composition root
// (cmd/authz/main.go) is responsible for always passing it the writer
// pool; this test proves that, once wired that way, closing the
// reader pool has no effect on it.
func TestAuthCodeRepository_FixedToWriterPool_UnaffectedByReaderPoolClose(t *testing.T) {
	writerDB := testsupport.OpenTestDB(t)
	readerDB := testsupport.OpenTestDB(t)
	testsupport.TruncateTable(t, writerDB, "authorization_codes")

	// Per this Spec's fixed wiring decision, authcode is always
	// constructed with the writer pool, never the reader pool.
	authCodes := postgres.NewAuthCodeRepository(writerDB)
	ctx := context.Background()

	ac := newPoolRoutingAuthCode(t)
	if err := authCodes.Save(ctx, ac); err != nil {
		t.Fatalf("Save() via writer pool unexpected error: %v", err)
	}

	if err := readerDB.Close(); err != nil {
		t.Fatalf("close reader pool: %v", err)
	}

	// FindByCode/Consume must be entirely unaffected by the reader
	// pool's closure, since this repository never touches it.
	got, err := authCodes.FindByCode(ctx, ac.Code())
	if err != nil {
		t.Fatalf("FindByCode() unexpected error after closing the (unrelated) reader pool: %v", err)
	}
	if got.Code() != ac.Code() {
		t.Errorf("FindByCode() returned code %v, want %v", got.Code(), ac.Code())
	}

	if err := authCodes.Consume(ctx, ac.Code()); err != nil {
		t.Errorf("Consume() unexpected error after closing the (unrelated) reader pool: %v", err)
	}
}
