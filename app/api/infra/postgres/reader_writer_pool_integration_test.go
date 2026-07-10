//go:build integration

// reader_writer_pool_integration_test.go is SPEC-010's integration
// suite for infra/postgres's writer/reader pool split
// (docs/plans/SPEC-010-plan.md フェーズ1): postgres.OpenPair's pool-
// sharing/opening decision, and proof that task.Reader/task.Writer
// implementations (postgres.NewTaskReader/postgres.NewTaskWriter) each
// route through their own *sql.DB pool, not the other's.
//
// As of the TDD "red" phase, infra/postgres has neither OpenPair nor
// NewTaskReader/NewTaskWriter yet: this file is written against their
// *planned* signatures (SPEC-010-plan.md "固定された対象シグネチャ") and
// therefore intentionally fails to compile with -tags=integration
// until impl-db lands infra/postgres/db.go's OpenPair and
// infra/postgres/task_reader.go / task_writer.go. That is expected and
// does not affect the default (untagged) build/vet/test, which never
// parses this file.
//
// Per the plan's "別ホスト reader の再現" note, the pool-routing proof
// below deliberately avoids requiring a second live database: it opens
// two independent *sql.DB pools (via the existing openTestDB helper,
// called twice) against the *same* reachable Postgres instance, and
// proves routing by closing one pool and observing that only the
// operations expected to use it start failing.
package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
)

// testConfig builds a postgres.Config from the same discrete DB_*
// environment variables (and defaults) openTestDB/testDSN use, so
// OpenPair-focused tests below can pass a postgres.Config directly
// instead of round-tripping through a DSN string.
func testConfig() postgres.Config {
	env := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	return postgres.Config{
		Host:     env("DB_HOST", "127.0.0.1"),
		Port:     env("DB_PORT", "5432"),
		Name:     env("DB_NAME", "api"),
		User:     env("DB_USER", "app"),
		Password: env("DB_PASSWORD", "app"),
		SSLMode:  env("DB_SSLMODE", "disable"),
	}
}

// TestOpenPair_SharesPoolWhenConfigEqual covers SPEC-010 R3 and the
// non-functional "二重に開かない" requirement: OpenPair called with an
// identical writer/reader Config must not open a second *sql.DB -- the
// returned reader pointer must be the exact same *sql.DB as writer.
func TestOpenPair_SharesPoolWhenConfigEqual(t *testing.T) {
	if os.Getenv("DB_HOST") == "" {
		t.Skip("DB_HOST not set; skipping infra/postgres integration test (see docs/plans/SPEC-005-plan.md §0)")
	}
	cfg := testConfig()
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
// surface as an error, with no usable *sql.DB handles returned -- so a
// caller can never observe a "successful" OpenPair silently downgraded
// to sharing the writer pool when the caller actually asked for a
// distinct one.
//
// This also indirectly proves the "differing Config -> attempt a
// second Open" branch executes at all: if OpenPair instead silently
// reused the writer pool whenever opening the reader is inconvenient,
// this call would succeed instead of failing.
func TestOpenPair_DifferentReaderConfig_FailsWithoutLeakingWriter(t *testing.T) {
	if os.Getenv("DB_HOST") == "" {
		t.Skip("DB_HOST not set; skipping infra/postgres integration test (see docs/plans/SPEC-005-plan.md §0)")
	}
	writerCfg := testConfig()
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

// TestTaskReaderWriter_PoolRouting_CloseVisualizes is SPEC-010 R2's
// core proof, using the plan's "close で可視化" technique: two
// independent *sql.DB pools opened against the same reachable database
// stand in for a writer pool and a (would-be-replica) reader pool.
// postgres.NewTaskWriter/NewTaskReader are each bound to one pool only;
// closing one pool must break exactly the operations documented as
// routed through it (task.Writer.Save via the writer pool,
// task.Reader.FindByID via the reader pool) while leaving the other
// pool's operations unaffected.
func TestTaskReaderWriter_PoolRouting_CloseVisualizes(t *testing.T) {
	t.Run("closing the reader pool fails reads but not writes", func(t *testing.T) {
		writerDB := openTestDB(t)
		readerDB := openTestDB(t)
		truncateTasks(t, writerDB)

		writer := postgres.NewTaskWriter(writerDB)
		reader := postgres.NewTaskReader(readerDB)
		ctx := context.Background()

		tk := task.New(mustIntegrationTitle(t, "reader pool routing"), task.PriorityMedium)
		if err := writer.Save(ctx, tk); err != nil {
			t.Fatalf("Save() via writer pool unexpected error: %v", err)
		}

		// Sanity: before severing it, the reader pool can see what the
		// writer just committed (same underlying database).
		if _, err := reader.FindByID(ctx, tk.ID()); err != nil {
			t.Fatalf("FindByID() via reader pool unexpected error before closing: %v", err)
		}

		if err := readerDB.Close(); err != nil {
			t.Fatalf("close reader pool: %v", err)
		}

		if _, err := reader.FindByID(ctx, tk.ID()); err == nil {
			t.Error("FindByID() via a closed reader pool succeeded, want an error (reads must route through the reader pool)")
		}

		// The writer pool is a fully independent *sql.DB; closing the
		// reader pool must not affect it.
		tk2 := task.New(mustIntegrationTitle(t, "reader pool routing 2"), task.PriorityMedium)
		if err := writer.Save(ctx, tk2); err != nil {
			t.Errorf("Save() via writer pool unexpected error after closing the reader pool: %v", err)
		}
	})

	t.Run("closing the writer pool fails writes but not reads", func(t *testing.T) {
		writerDB := openTestDB(t)
		readerDB := openTestDB(t)
		truncateTasks(t, writerDB)

		writer := postgres.NewTaskWriter(writerDB)
		reader := postgres.NewTaskReader(readerDB)
		ctx := context.Background()

		tk := task.New(mustIntegrationTitle(t, "writer pool routing"), task.PriorityMedium)
		if err := writer.Save(ctx, tk); err != nil {
			t.Fatalf("Save() via writer pool unexpected error: %v", err)
		}

		if err := writerDB.Close(); err != nil {
			t.Fatalf("close writer pool: %v", err)
		}

		tk2 := task.New(mustIntegrationTitle(t, "writer pool routing 2"), task.PriorityMedium)
		if err := writer.Save(ctx, tk2); err == nil {
			t.Error("Save() via a closed writer pool succeeded, want an error (writes must route through the writer pool)")
		}

		// The reader pool is independent; it can still read the row the
		// writer committed before being closed.
		if _, err := reader.FindByID(ctx, tk.ID()); err != nil {
			t.Errorf("FindByID() via reader pool unexpected error after closing the writer pool: %v", err)
		}
	})
}
