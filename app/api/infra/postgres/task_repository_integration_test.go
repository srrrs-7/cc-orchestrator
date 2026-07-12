// Package postgres_test holds the SPEC-005 test suite for
// infra/postgres. As of SPEC-013 it runs as part of the default
// `make test` / `make check`, against the dedicated `api_test`
// database (see infra/postgres/testsupport), which `app/migrator`
// creates and migrates ahead of time (this suite does not run
// migrations itself).
package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/repotest"
)

// TestTaskRepository_Contract runs the behavioral contract shared by
// every task.Repository implementation (SPEC-005 R1 / SPEC-011 R3)
// against a real Postgres-backed task.Repository.
func TestTaskRepository_Contract(t *testing.T) {
	repotest.RunTaskRepositoryContract(t, func(t *testing.T) task.Repository {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTasks(t, db)
		return postgres.NewTaskRepository(db)
	})
}

// TestTaskRepository_UniqueTitle_ViolatesConstraint is a Postgres-only
// test: unlike infra/memory (which silently allows two distinct Tasks
// to share a Title -- see docs/plans/SPEC-005-plan.md §6.1 R-a), the
// tasks.title column is expected to carry a UNIQUE constraint. This is
// deliberately NOT part of the shared repotest contract, since
// infra/memory does not (and per R-a, is not required to) exhibit
// this behavior.
//
// Expected mapping (impl-db): the Postgres unique_violation (SQLSTATE
// 23505) on tasks.title should be translated to the domain's existing
// task.ErrDuplicateTitle sentinel -- the same one route/response.go
// and service.TaskService already branch on via the application-level
// task.DuplicateChecker pre-check -- so infra/postgres's
// defense-in-depth failure mode matches the rest of the codebase's
// error taxonomy instead of leaking a raw database/sql or pgx error.
func TestTaskRepository_UniqueTitle_ViolatesConstraint(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	testsupport.TruncateTasks(t, db)
	repo := postgres.NewTaskRepository(db)
	ctx := context.Background()

	title, err := task.NewTitle("duplicate title")
	if err != nil {
		t.Fatalf("NewTitle() unexpected error: %v", err)
	}

	first := task.New(title, task.PriorityMedium)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save(first) unexpected error: %v", err)
	}

	second := task.New(title, task.PriorityMedium) // distinct ID, same Title
	err = repo.Save(ctx, second)
	if err == nil {
		t.Fatal("Save(second) with a duplicate title succeeded, want a UNIQUE constraint violation")
	}
	if !errors.Is(err, task.ErrDuplicateTitle) {
		t.Errorf("Save(second) error = %v, want wrapping %v", err, task.ErrDuplicateTitle)
	}
	// ISSUE-018: the unique_violation must also be recognized as the
	// *task.ConflictError category (HTTP 409), not just the bare
	// ErrDuplicateTitle sentinel, so route's errors.As type switch maps
	// it correctly.
	var ce *task.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("Save(second) error = %v, want errors.As(&task.ConflictError{}) = true", err)
	}
}

// TestTaskRepository_ErrorCategories_ISSUE018 covers ISSUE-018's
// category-type mapping for infra/postgres:
//   - not-found -> *task.NotFoundError (in addition to the shared
//     repotest contract's errors.Is(ErrNotFound) coverage);
//   - a corrupt row (bypassing domain validation at the DB layer) and
//     a forced driver failure both -> *task.DBError, and -- the
//     regression this Issue exists to prevent -- must NOT also
//     satisfy errors.As(&task.ValidationError{}). taskFromRow
//     deliberately severs an inner *task.ValidationError from the
//     Unwrap chain via %v (not %w) precisely so a corrupt DB row
//     surfaces as HTTP 500, not HTTP 400 (docs/plans/ISSUE-018-plan.md
//     "リスク / 未確定事項").
func TestTaskRepository_ErrorCategories_ISSUE018(t *testing.T) {
	t.Run("FindByID not found maps to *task.NotFoundError", func(t *testing.T) {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTasks(t, db)
		repo := postgres.NewTaskRepository(db)

		_, err := repo.FindByID(context.Background(), task.NewID())

		var nfe *task.NotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("FindByID() error = %v, want errors.As(&task.NotFoundError{}) = true", err)
		}
		if !errors.Is(err, task.ErrNotFound) {
			t.Errorf("FindByID() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("corrupt row (empty title bypassing domain validation) maps to *task.DBError, not *task.ValidationError", func(t *testing.T) {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTasks(t, db)
		ctx := context.Background()

		// Insert a row directly, bypassing task.NewTitle's
		// empty-title rejection: the tasks.title column only enforces
		// NOT NULL, not non-empty, so this models a row corrupted by
		// something other than this application (e.g. a manual data
		// fix, a different writer).
		id := task.NewID().String()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO tasks (id, title, status, priority, created_at, updated_at) VALUES ($1, '', 'todo', 'medium', now(), now())`,
			id,
		); err != nil {
			t.Fatalf("setup: insert corrupt row: %v", err)
		}

		repo := postgres.NewTaskRepository(db)
		parsedID, err := task.ParseID(id)
		if err != nil {
			t.Fatalf("setup: ParseID(%q) unexpected error: %v", id, err)
		}

		_, err = repo.FindByID(ctx, parsedID)
		if err == nil {
			t.Fatal("FindByID() on a corrupt row succeeded, want an error")
		}

		var dbErr *task.DBError
		if !errors.As(err, &dbErr) {
			t.Fatalf("FindByID() error = %v, want errors.As(&task.DBError{}) = true", err)
		}
		var ve *task.ValidationError
		if errors.As(err, &ve) {
			t.Errorf("FindByID() error = %v, want errors.As(&task.ValidationError{}) = false (corrupt row must map to 500, not 400)", err)
		}
	})

	t.Run("forced driver failure (closed connection) maps to *task.DBError", func(t *testing.T) {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTasks(t, db)
		repo := postgres.NewTaskRepository(db)
		if err := db.Close(); err != nil {
			t.Fatalf("setup: close db: %v", err)
		}

		// SPEC-008 replaced task.Repository.FindAll with ListPage (items
		// + total); a closed connection must still surface as a
		// *task.DBError, whichever of the two queries (CountTasks,
		// ListTasksPage) the driver failure is observed on first.
		_, _, err := repo.ListPage(context.Background(), mustPage(t, nil, nil))
		if err == nil {
			t.Fatal("ListPage() on a closed connection succeeded, want an error")
		}

		var dbErr *task.DBError
		if !errors.As(err, &dbErr) {
			t.Fatalf("ListPage() error = %v, want errors.As(&task.DBError{}) = true", err)
		}
		var ve *task.ValidationError
		if errors.As(err, &ve) {
			t.Errorf("ListPage() error = %v, want errors.As(&task.ValidationError{}) = false", err)
		}
	})
}

// mustPage builds a task.Page via task.NewPage, failing the test on
// any validation error. Mirrors infra/repotest.mustPage /
// infra/memory's mustTestPage, duplicated here (rather than exported
// from repotest) since it is a one-line, test-only convenience and
// this file must not grow repotest's exported surface.
func mustPage(t *testing.T, limit, offset *int) task.Page {
	t.Helper()
	page, err := task.NewPage(limit, offset)
	if err != nil {
		t.Fatalf("NewPage() unexpected error: %v", err)
	}
	return page
}

// intPtr returns a pointer to i, for building *int limit/offset
// arguments to task.NewPage in table-driven tests below.
func intPtr(i int) *int {
	return &i
}

// newIntegrationTaskAt builds a Task with an explicit id and
// createdAt (via task.Reconstruct) instead of task.New's
// time.Now(), so ordering/windowing tests can control the
// (created_at, id) sort key deterministically -- including forcing a
// created_at tie broken only by id -- against a real Postgres
// instance, mirroring infra/memory's own ListPage ordering tests.
func newIntegrationTaskAt(t *testing.T, id, title string, createdAt time.Time) *task.Task {
	t.Helper()
	parsedID, err := task.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%q) unexpected error: %v", id, err)
	}
	tt, err := task.NewTitle(title)
	if err != nil {
		t.Fatalf("NewTitle(%q) unexpected error: %v", title, err)
	}
	return task.Reconstruct(parsedID, tt, task.StatusTodo, task.PriorityMedium, createdAt, createdAt)
}

// TestTaskRepository_ListPage_Boundaries covers SPEC-008 R1/R2/R5
// against a real Postgres instance: the LIMIT/OFFSET window (`db/
// queries/tasks.sql`'s ListTasksPage), CountTasks's total independent
// of that window, an offset at or beyond total yielding empty items
// (not an error), and the `ORDER BY created_at, id` stable ordering
// (ties on created_at broken by id) -- the same guarantees
// infra/memory provides, proving R5 ("infra/postgres の振る舞いは
// infra/memory と同じ") for the paginated read path specifically.
func TestTaskRepository_ListPage_Boundaries(t *testing.T) {
	t.Run("offset beyond total yields empty items and the store's full total", func(t *testing.T) {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTasks(t, db)
		repo := postgres.NewTaskRepository(db)
		ctx := context.Background()

		tk1 := task.New(mustIntegrationTitle(t, "buy milk"), task.PriorityMedium)
		tk2 := task.New(mustIntegrationTitle(t, "walk dog"), task.PriorityMedium)
		if err := repo.Save(ctx, tk1); err != nil {
			t.Fatalf("Save(tk1) unexpected error: %v", err)
		}
		if err := repo.Save(ctx, tk2); err != nil {
			t.Fatalf("Save(tk2) unexpected error: %v", err)
		}

		items, total, err := repo.ListPage(ctx, mustPage(t, nil, intPtr(5)))
		if err != nil {
			t.Fatalf("ListPage() unexpected error: %v", err)
		}
		if len(items) != 0 {
			t.Fatalf("ListPage() with offset beyond total = %d items, want 0", len(items))
		}
		if total != 2 {
			t.Fatalf("ListPage() total = %d, want 2 (total must reflect the store regardless of offset)", total)
		}
	})

	t.Run("orders by created_at then id ascending, ties broken by id", func(t *testing.T) {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTasks(t, db)
		repo := postgres.NewTaskRepository(db)
		ctx := context.Background()
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		tk1 := newIntegrationTaskAt(t, "id-1", "first, strictly earlier", base)
		// tk2 and tk3 share the same created_at; only id breaks the tie.
		tk3 := newIntegrationTaskAt(t, "id-3", "tie, higher id", base.Add(time.Second))
		tk2 := newIntegrationTaskAt(t, "id-2", "tie, lower id", base.Add(time.Second))

		// Save out of sort order to prove ListPage's ORDER BY sorts
		// explicitly rather than happening to preserve insertion order.
		for _, tk := range []*task.Task{tk3, tk1, tk2} {
			if err := repo.Save(ctx, tk); err != nil {
				t.Fatalf("Save(%v) unexpected error: %v", tk.ID(), err)
			}
		}

		items, total, err := repo.ListPage(ctx, mustPage(t, nil, nil))
		if err != nil {
			t.Fatalf("ListPage() unexpected error: %v", err)
		}
		if total != 3 {
			t.Fatalf("ListPage() total = %d, want 3", total)
		}

		wantOrder := []string{"id-1", "id-2", "id-3"}
		if len(items) != len(wantOrder) {
			t.Fatalf("ListPage() = %d items, want %d", len(items), len(wantOrder))
		}
		gotOrder := make([]string, len(items))
		for i, tk := range items {
			gotOrder[i] = tk.ID().String()
		}
		for i := range wantOrder {
			if gotOrder[i] != wantOrder[i] {
				t.Errorf("ListPage() order = %v, want %v (created_at, id ascending)", gotOrder, wantOrder)
			}
		}
	})

	t.Run("limit slices the requested window across successive pages", func(t *testing.T) {
		db := testsupport.OpenTestDB(t)
		testsupport.TruncateTasks(t, db)
		repo := postgres.NewTaskRepository(db)
		ctx := context.Background()
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		ids := []string{"id-1", "id-2", "id-3", "id-4", "id-5"}
		for i, id := range ids {
			tk := newIntegrationTaskAt(t, id, id, base.Add(time.Duration(i)*time.Second))
			if err := repo.Save(ctx, tk); err != nil {
				t.Fatalf("Save(%s) unexpected error: %v", id, err)
			}
		}

		tests := []struct {
			name    string
			limit   int
			offset  int
			wantIDs []string
		}{
			{name: "first page", limit: 2, offset: 0, wantIDs: []string{"id-1", "id-2"}},
			{name: "second page", limit: 2, offset: 2, wantIDs: []string{"id-3", "id-4"}},
			{name: "final partial page (fewer rows than limit remain)", limit: 2, offset: 4, wantIDs: []string{"id-5"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				items, total, err := repo.ListPage(ctx, mustPage(t, intPtr(tt.limit), intPtr(tt.offset)))
				if err != nil {
					t.Fatalf("ListPage() unexpected error: %v", err)
				}
				if total != len(ids) {
					t.Fatalf("ListPage() total = %d, want %d", total, len(ids))
				}

				gotIDs := make([]string, len(items))
				for i, tk := range items {
					gotIDs[i] = tk.ID().String()
				}
				if len(gotIDs) != len(tt.wantIDs) {
					t.Fatalf("ListPage() = %v, want %v", gotIDs, tt.wantIDs)
				}
				for i := range tt.wantIDs {
					if gotIDs[i] != tt.wantIDs[i] {
						t.Errorf("ListPage() = %v, want %v", gotIDs, tt.wantIDs)
					}
				}
			})
		}
	})
}

// mustIntegrationTitle builds a task.Title, failing the test on any
// validation error.
func mustIntegrationTitle(t *testing.T, s string) task.Title {
	t.Helper()
	title, err := task.NewTitle(s)
	if err != nil {
		t.Fatalf("NewTitle(%q) unexpected error: %v", s, err)
	}
	return title
}
