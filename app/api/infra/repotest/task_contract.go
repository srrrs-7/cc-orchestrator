// Package repotest provides a shared "behavioral contract" test
// suite for task.Repository implementations (SPEC-005). The domain
// layer's Repository interface (domain/task/repository.go) is the
// single source of truth for what a task.Repository must do;
// RunTaskRepositoryContract exercises that contract once, so
// infra/memory and (once implemented) infra/postgres can both be
// proven to behave identically without duplicating test logic
// per-implementation.
//
// This file carries no build tag: it must compile and be usable both
// by the default (untagged) build -- exercised today against
// infra/memory -- and by the "integration" build (see
// infra/postgres/task_repository_integration_test.go) once
// infra/postgres exists. It must therefore not depend on anything
// beyond the standard library and the task domain package.
package repotest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// NewTaskRepository constructs a task.Repository backed by a clean,
// empty store, ready for a single subtest.
//
// Implementations MUST return a repository whose store is empty every
// time this is called (a fresh in-memory map for infra/memory; a
// truncated table for infra/postgres), so that RunTaskRepositoryContract's
// subtests never observe data left behind by another subtest. See
// infra/memory/task_repository_contract_test.go and (once it exists)
// infra/postgres/task_repository_integration_test.go for the two
// concrete factories.
type NewTaskRepository func(t *testing.T) task.Repository

// RunTaskRepositoryContract runs the behavioral contract shared by
// every task.Repository implementation (SPEC-005 R1 / SPEC-008 R5:
// infra/postgres must behave identically to infra/memory for Save /
// FindByID / FindByTitle / ListPage).
//
// Deliberately NOT covered here (see docs/plans/SPEC-005-plan.md
// §6.1 R-a): UNIQUE(title) enforcement. infra/memory silently allows
// saving two distinct Tasks that share a Title, while a Postgres
// UNIQUE constraint on tasks.title is expected to reject it. That
// asymmetry is intentionally out of scope for a *shared* contract and
// is instead verified by a Postgres-only test in infra/postgres.
func RunTaskRepositoryContract(t *testing.T, newRepo NewTaskRepository) {
	t.Helper()

	t.Run("Save then FindByID round-trips every field", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		tk := task.New(mustTitle(t, "buy milk"), task.PriorityHigh)

		if err := repo.Save(ctx, tk); err != nil {
			t.Fatalf("Save() unexpected error: %v", err)
		}

		got, err := repo.FindByID(ctx, tk.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		assertSameTask(t, got, tk)
	})

	t.Run("Save called again with the same id upserts (updates in place, does not duplicate)", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		originalTitle := mustTitle(t, "buy milk")
		tk := task.New(originalTitle, task.PriorityLow)
		if err := repo.Save(ctx, tk); err != nil {
			t.Fatalf("first Save() unexpected error: %v", err)
		}

		renamedTitle := mustTitle(t, "buy oat milk")
		tk.Rename(renamedTitle)
		tk.ChangePriority(task.PriorityHigh)
		if err := repo.Save(ctx, tk); err != nil {
			t.Fatalf("second Save() unexpected error: %v", err)
		}

		got, err := repo.FindByID(ctx, tk.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		if got.Title() != renamedTitle {
			t.Errorf("Title() = %v, want %v (second Save() must update in place)", got.Title(), renamedTitle)
		}
		if got.Priority() != task.PriorityHigh {
			t.Errorf("Priority() = %v, want %v (second Save() must update in place)", got.Priority(), task.PriorityHigh)
		}

		items, total, err := repo.ListPage(ctx, mustPage(t, nil, nil))
		if err != nil {
			t.Fatalf("ListPage() unexpected error: %v", err)
		}
		if total != 1 {
			t.Fatalf("ListPage() total = %d, want 1 (second Save() with the same id must not create a duplicate row)", total)
		}
		if len(items) != 1 {
			t.Fatalf("ListPage() = %d items, want 1 (second Save() with the same id must not create a duplicate row)", len(items))
		}

		if _, err := repo.FindByTitle(ctx, originalTitle); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("FindByTitle(original, now-stale title) error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("FindByID for an id that was never saved returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.FindByID(context.Background(), task.NewID())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("FindByID() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("FindByTitle finds a saved task by its exact title", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		tk := task.New(mustTitle(t, "walk dog"), task.PriorityMedium)
		if err := repo.Save(ctx, tk); err != nil {
			t.Fatalf("Save() unexpected error: %v", err)
		}

		got, err := repo.FindByTitle(ctx, tk.Title())
		if err != nil {
			t.Fatalf("FindByTitle() unexpected error: %v", err)
		}
		if got.ID() != tk.ID() {
			t.Errorf("ID() = %v, want %v", got.ID(), tk.ID())
		}
	})

	t.Run("FindByTitle for a title that was never saved returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.FindByTitle(context.Background(), mustTitle(t, "never saved"))
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("FindByTitle() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("ListPage on an empty repository returns zero items and zero total without error", func(t *testing.T) {
		repo := newRepo(t)
		items, total, err := repo.ListPage(context.Background(), mustPage(t, nil, nil))
		if err != nil {
			t.Fatalf("ListPage() unexpected error: %v", err)
		}
		if len(items) != 0 {
			t.Fatalf("ListPage() = %d items, want 0", len(items))
		}
		if total != 0 {
			t.Fatalf("ListPage() total = %d, want 0", total)
		}
	})

	t.Run("ListPage returns exactly every saved task and total when the page covers all of them", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		tk1 := task.New(mustTitle(t, "buy milk"), task.PriorityLow)
		tk2 := task.New(mustTitle(t, "walk dog"), task.PriorityHigh)
		if err := repo.Save(ctx, tk1); err != nil {
			t.Fatalf("Save(tk1) unexpected error: %v", err)
		}
		if err := repo.Save(ctx, tk2); err != nil {
			t.Fatalf("Save(tk2) unexpected error: %v", err)
		}

		items, total, err := repo.ListPage(ctx, mustPage(t, nil, nil))
		if err != nil {
			t.Fatalf("ListPage() unexpected error: %v", err)
		}
		if total != 2 {
			t.Fatalf("ListPage() total = %d, want 2", total)
		}
		if len(items) != 2 {
			t.Fatalf("ListPage() = %d items, want 2", len(items))
		}

		gotIDs := make(map[task.ID]bool, len(items))
		for _, tk := range items {
			gotIDs[tk.ID()] = true
		}
		if !gotIDs[tk1.ID()] || !gotIDs[tk2.ID()] {
			t.Errorf("ListPage() ids = %v, want to contain both %v and %v", gotIDs, tk1.ID(), tk2.ID())
		}
	})

	t.Run("Save then FindByID round-trips a boundary-length (100 rune) title", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		longTitle := mustTitle(t, longRuneString(100))
		tk := task.New(longTitle, task.PriorityMedium)
		if err := repo.Save(ctx, tk); err != nil {
			t.Fatalf("Save() unexpected error: %v", err)
		}

		got, err := repo.FindByID(ctx, tk.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		if got.Title() != longTitle {
			t.Errorf("Title() round-trip mismatch for a 100-rune title: got %q, want %q", got.Title(), longTitle)
		}
	})
}

func mustTitle(t *testing.T, s string) task.Title {
	t.Helper()
	title, err := task.NewTitle(s)
	if err != nil {
		t.Fatalf("NewTitle(%q) unexpected error: %v", s, err)
	}
	return title
}

// mustPage builds a task.Page via task.NewPage, failing the test on
// any validation error. A nil limit/offset default to
// task.DefaultLimit/0 (task.NewPage), which comfortably covers every
// fixture this contract saves (well under task.DefaultLimit), letting
// subtests exercise ListPage without needing to reason about paging
// math themselves.
func mustPage(t *testing.T, limit, offset *int) task.Page {
	t.Helper()
	page, err := task.NewPage(limit, offset)
	if err != nil {
		t.Fatalf("NewPage() unexpected error: %v", err)
	}
	return page
}

// longRuneString returns a string of exactly n runes, used to probe
// Title's documented boundary (domain/task/title.go: maxTitleRunes =
// 100).
func longRuneString(n int) string {
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = 'a'
	}
	return string(runes)
}

// assertSameTask compares every observable field of got against
// want. CreatedAt/UpdatedAt are compared with microsecond truncation:
// Postgres's timestamptz column has microsecond precision, while
// Go's time.Now() carries nanoseconds, so an exact time.Time ==
// comparison would spuriously fail once this contract is exercised
// against a real database (it does not affect infra/memory today,
// which never truncates, but keeps this file usable unmodified by
// infra/postgres later).
func assertSameTask(t *testing.T, got, want *task.Task) {
	t.Helper()
	if got.ID() != want.ID() {
		t.Errorf("ID() = %v, want %v", got.ID(), want.ID())
	}
	if got.Title() != want.Title() {
		t.Errorf("Title() = %v, want %v", got.Title(), want.Title())
	}
	if got.Status() != want.Status() {
		t.Errorf("Status() = %v, want %v", got.Status(), want.Status())
	}
	if got.Priority() != want.Priority() {
		t.Errorf("Priority() = %v, want %v", got.Priority(), want.Priority())
	}
	if !got.CreatedAt().Truncate(time.Microsecond).Equal(want.CreatedAt().Truncate(time.Microsecond)) {
		t.Errorf("CreatedAt() = %v, want %v", got.CreatedAt(), want.CreatedAt())
	}
	if !got.UpdatedAt().Truncate(time.Microsecond).Equal(want.UpdatedAt().Truncate(time.Microsecond)) {
		t.Errorf("UpdatedAt() = %v, want %v", got.UpdatedAt(), want.UpdatedAt())
	}
}
