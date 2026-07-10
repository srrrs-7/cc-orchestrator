package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/memory"
)

func newTestTask(t *testing.T, title string) *task.Task {
	t.Helper()
	tt, err := task.NewTitle(title)
	if err != nil {
		t.Fatalf("NewTitle(%q) unexpected error: %v", title, err)
	}
	return task.New(tt, task.PriorityMedium)
}

func TestTaskRepository_SaveAndFindByID(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")

	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got.ID() != tk.ID() {
		t.Errorf("ID() = %v, want %v", got.ID(), tk.ID())
	}
	if got.Title() != tk.Title() {
		t.Errorf("Title() = %v, want %v", got.Title(), tk.Title())
	}
	if got.Status() != tk.Status() {
		t.Errorf("Status() = %v, want %v", got.Status(), tk.Status())
	}
	// R1: clone (used by Save/FindByID under the hood) preserves priority.
	if got.Priority() != tk.Priority() {
		t.Errorf("Priority() = %v, want %v", got.Priority(), tk.Priority())
	}
}

// TestTaskRepository_SaveAndFindByID_PreservesPriority is a dedicated
// R1 regression: a non-default priority set at construction time must
// survive the Save -> FindByID round trip through clone()/Reconstruct.
func TestTaskRepository_SaveAndFindByID_PreservesPriority(t *testing.T) {
	repo := memory.NewTaskRepository()
	title, err := task.NewTitle("buy milk")
	if err != nil {
		t.Fatalf("NewTitle() unexpected error: %v", err)
	}
	tk := task.New(title, task.PriorityHigh)

	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got.Priority() != task.PriorityHigh {
		t.Errorf("Priority() = %v, want %v", got.Priority(), task.PriorityHigh)
	}
}

func TestTaskRepository_FindByID_NotFound(t *testing.T) {
	repo := memory.NewTaskRepository()

	_, err := repo.FindByID(context.Background(), task.NewID())
	if !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("FindByID() error = %v, want wrapping %v", err, task.ErrNotFound)
	}
}

func TestTaskRepository_FindByTitle(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")
	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	t.Run("existing title is found", func(t *testing.T) {
		got, err := repo.FindByTitle(context.Background(), tk.Title())
		if err != nil {
			t.Fatalf("FindByTitle() unexpected error: %v", err)
		}
		if got.ID() != tk.ID() {
			t.Errorf("ID() = %v, want %v", got.ID(), tk.ID())
		}
	})

	t.Run("unknown title is not found", func(t *testing.T) {
		other, err := task.NewTitle("walk dog")
		if err != nil {
			t.Fatalf("NewTitle() unexpected error: %v", err)
		}

		_, err = repo.FindByTitle(context.Background(), other)
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("FindByTitle() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

func mustTestPage(t *testing.T, limit, offset *int) task.Page {
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

// newTestTaskAt builds a Task with an explicit id and createdAt (via
// task.Reconstruct) instead of task.New's time.Now(), so ordering/
// windowing tests can control the (created_at, id) sort key
// deterministically -- including forcing a created_at tie broken only
// by id.
func newTestTaskAt(t *testing.T, id, title string, createdAt time.Time) *task.Task {
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

// TestTaskRepository_ListPage_OffsetBeyondTotal covers SPEC-008's
// documented boundary: an offset at or beyond the store's total
// yields an empty items slice (not an error), while total still
// reports the store's full count.
func TestTaskRepository_ListPage_OffsetBeyondTotal(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk1 := newTestTask(t, "buy milk")
	tk2 := newTestTask(t, "walk dog")
	if err := repo.Save(context.Background(), tk1); err != nil {
		t.Fatalf("Save(tk1) unexpected error: %v", err)
	}
	if err := repo.Save(context.Background(), tk2); err != nil {
		t.Fatalf("Save(tk2) unexpected error: %v", err)
	}

	items, total, err := repo.ListPage(context.Background(), mustTestPage(t, nil, intPtr(5)))
	if err != nil {
		t.Fatalf("ListPage() unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("ListPage() with offset beyond total = %d items, want 0", len(items))
	}
	if total != 2 {
		t.Fatalf("ListPage() total = %d, want 2 (total must reflect the store regardless of offset)", total)
	}
}

// TestTaskRepository_ListPage_OrdersByCreatedAtThenID covers SPEC-008
// R5: results are ordered deterministically by created_at ascending,
// with ties broken by id ascending -- and this holds regardless of
// Save order (the backing map has no inherent order), matching
// infra/postgres's `ORDER BY created_at, id`.
func TestTaskRepository_ListPage_OrdersByCreatedAtThenID(t *testing.T) {
	repo := memory.NewTaskRepository()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tk1 := newTestTaskAt(t, "id-1", "first, strictly earlier", base)
	// tk2 and tk3 share the same createdAt; only id breaks the tie.
	tk3 := newTestTaskAt(t, "id-3", "tie, higher id", base.Add(time.Second))
	tk2 := newTestTaskAt(t, "id-2", "tie, lower id", base.Add(time.Second))

	// Save out of sort order to prove ListPage sorts explicitly
	// rather than happening to preserve insertion/map iteration order.
	for _, tk := range []*task.Task{tk3, tk1, tk2} {
		if err := repo.Save(context.Background(), tk); err != nil {
			t.Fatalf("Save(%v) unexpected error: %v", tk.ID(), err)
		}
	}

	items, total, err := repo.ListPage(context.Background(), mustTestPage(t, nil, nil))
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
}

// TestTaskRepository_ListPage_LimitSlicesRequestedWindow covers
// SPEC-008 R1/R5: successive limit/offset pages (including a final,
// partial page) each return exactly the expected slice of the
// deterministic (created_at, id) order, with no duplicate or skipped
// row across page boundaries.
func TestTaskRepository_ListPage_LimitSlicesRequestedWindow(t *testing.T) {
	repo := memory.NewTaskRepository()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := []string{"id-1", "id-2", "id-3", "id-4", "id-5"}
	for i, id := range ids {
		tk := newTestTaskAt(t, id, id, base.Add(time.Duration(i)*time.Second))
		if err := repo.Save(context.Background(), tk); err != nil {
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
			page := mustTestPage(t, intPtr(tt.limit), intPtr(tt.offset))
			items, total, err := repo.ListPage(context.Background(), page)
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
}

func TestTaskRepository_ListPage(t *testing.T) {
	repo := memory.NewTaskRepository()

	items, total, err := repo.ListPage(context.Background(), mustTestPage(t, nil, nil))
	if err != nil {
		t.Fatalf("ListPage() unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("ListPage() on empty repo = %d items, want 0", len(items))
	}
	if total != 0 {
		t.Fatalf("ListPage() total on empty repo = %d, want 0", total)
	}

	tk1 := newTestTask(t, "buy milk")
	tk2 := newTestTask(t, "walk dog")
	if err := repo.Save(context.Background(), tk1); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}
	if err := repo.Save(context.Background(), tk2); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	items, total, err = repo.ListPage(context.Background(), mustTestPage(t, nil, nil))
	if err != nil {
		t.Fatalf("ListPage() unexpected error: %v", err)
	}
	if total != 2 {
		t.Fatalf("ListPage() total = %d, want 2", total)
	}
	if len(items) != 2 {
		t.Fatalf("ListPage() = %d items, want 2", len(items))
	}

	ids := map[task.ID]bool{}
	for _, tk := range items {
		ids[tk.ID()] = true
	}
	if !ids[tk1.ID()] || !ids[tk2.ID()] {
		t.Errorf("ListPage() = %v, want to contain both %v and %v", ids, tk1.ID(), tk2.ID())
	}
}

// TestTaskRepository_Save_DoesNotAliasCallersTask verifies that Save
// stores a clone: mutating the caller's *task.Task after Save must
// not affect the repository's stored state.
func TestTaskRepository_Save_DoesNotAliasCallersTask(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")

	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	// Mutate the caller's task after it has been saved.
	if err := tk.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}

	got, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got.Status() != task.StatusTodo {
		t.Errorf("stored Status() = %v, want %v (mutation of caller's task leaked into repository)", got.Status(), task.StatusTodo)
	}
}

// TestTaskRepository_CanceledContext_ReturnsDBError covers ISSUE-018:
// every method must translate a canceled context into a
// *task.DBError (HTTP 500 category), matching infra/postgres's own
// mapping of infrastructure-layer failures (db.md's "memory と
// postgres の振る舞い一致" requirement), instead of leaking the bare
// context.Canceled sentinel untyped.
func TestTaskRepository_CanceledContext_ReturnsDBError(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")
	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("setup Save() unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "Save",
			call: func() error { return repo.Save(ctx, tk) },
		},
		{
			name: "FindByID",
			call: func() error { _, err := repo.FindByID(ctx, tk.ID()); return err },
		},
		{
			name: "FindByTitle",
			call: func() error { _, err := repo.FindByTitle(ctx, tk.Title()); return err },
		},
		{
			name: "ListPage",
			call: func() error { _, _, err := repo.ListPage(ctx, mustTestPage(t, nil, nil)); return err },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("error = nil, want a *task.DBError")
			}

			var dbErr *task.DBError
			if !errors.As(err, &dbErr) {
				t.Fatalf("errors.As(err, &task.DBError{}) = false, want true (err = %v)", err)
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("errors.Is(err, context.Canceled) = false, want true (err = %v)", err)
			}
		})
	}
}

// TestTaskRepository_FindByID_ReturnsIndependentCopies verifies that
// the *task.Task returned by FindByID is a clone: mutating it must
// not affect the repository's stored state or subsequent reads.
func TestTaskRepository_FindByID_ReturnsIndependentCopies(t *testing.T) {
	repo := memory.NewTaskRepository()
	tk := newTestTask(t, "buy milk")
	if err := repo.Save(context.Background(), tk); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got1, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if err := got1.Start(); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}

	got2, err := repo.FindByID(context.Background(), tk.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	if got2.Status() != task.StatusTodo {
		t.Errorf("stored Status() = %v, want %v (mutation of returned task leaked into repository)", got2.Status(), task.StatusTodo)
	}
}
