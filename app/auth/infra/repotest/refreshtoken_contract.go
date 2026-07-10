package repotest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// NewRefreshTokenRepository constructs a refreshtoken.Repository
// backed by a clean, empty store, ready for a single subtest.
//
// Implementations MUST return a repository whose store is empty every
// time this is called (a fresh in-memory map for infra/memory; a
// truncated table for infra/postgres), so that
// RunRefreshTokenRepositoryContract's subtests never observe data
// left behind by another subtest. Mirrors
// repotest.NewAuthCodeRepository (authcode_contract.go).
type NewRefreshTokenRepository func(t *testing.T) refreshtoken.Repository

// RunRefreshTokenRepositoryContract runs the behavioral contract
// shared by every refreshtoken.Repository implementation (SPEC-006
// R4/R5/R8, docs/plans/SPEC-006-plan.md 付録 A): Save / FindByTokenHash
// round-trip every field including a consumed-but-unexpired row
// (needed for reuse detection), the atomic single-use + rotation
// mechanism (Rotate, including its ErrReused vs ErrNotFound
// precedence and behavior under concurrency), family-wide revocation
// (RevokeFamily), and TTL-based expiry (including lazy eviction) --
// all behave identically for infra/memory and infra/postgres.
//
// Real-time sleeps are never used to exercise TTL: every
// expiring/expired fixture is built via refreshtoken.Reconstruct with
// an expiresAt computed once, up front, relative to time.Now() (see
// newRefreshTokenExpiringAt), per testing.md "実時間 sleep に依存する
// テストを書かない". Mirrors repotest.RunAuthCodeRepositoryContract
// (authcode_contract.go).
func RunRefreshTokenRepositoryContract(t *testing.T, newRepo NewRefreshTokenRepository) {
	t.Helper()

	t.Run("Save then FindByTokenHash round-trips every field, Consumed()=false", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt, _ := newTestRefreshToken(t)

		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("Save() unexpected error: %v", err)
		}

		got, err := repo.FindByTokenHash(ctx, rt.TokenHash())
		if err != nil {
			t.Fatalf("FindByTokenHash() unexpected error: %v", err)
		}
		assertSameRefreshToken(t, got, rt)
		if got.Consumed() {
			t.Error("Consumed() = true for a freshly saved refresh token, want false")
		}
	})

	t.Run("FindByTokenHash for a hash that was never saved returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		unknown := refreshtoken.HashToken("never-issued-plaintext")

		if _, err := repo.FindByTokenHash(context.Background(), unknown); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Fatalf("FindByTokenHash() error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
		}
	})

	t.Run("FindByTokenHash for an expired row returns ErrNotFound and evicts it (lazy deletion)", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt := newRefreshTokenExpiringAt(t, -1*time.Minute)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if _, err := repo.FindByTokenHash(ctx, rt.TokenHash()); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Fatalf("first FindByTokenHash() error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
		}
		if _, err := repo.FindByTokenHash(ctx, rt.TokenHash()); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Fatalf("second FindByTokenHash() error = %v, want wrapping %v (entry must have been evicted by the first lookup)", err, refreshtoken.ErrNotFound)
		}
	})

	t.Run("FindByTokenHash returns a consumed-but-unexpired row (reuse detection needs to see it)", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt, _ := newTestRefreshToken(t)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}
		newRT, _ := rotateTestRefreshToken(t, rt)
		if err := repo.Rotate(ctx, rt.TokenHash(), newRT); err != nil {
			t.Fatalf("setup Rotate() unexpected error: %v", err)
		}

		got, err := repo.FindByTokenHash(ctx, rt.TokenHash())
		if err != nil {
			t.Fatalf("FindByTokenHash() on a consumed-but-unexpired row: unexpected error: %v, want nil (consumed-but-unexpired rows must still be found)", err)
		}
		if !got.Consumed() {
			t.Error("Consumed() = false, want true (the row was consumed by the prior Rotate)")
		}
	})

	t.Run("Rotate on an existing, non-expired, non-consumed token succeeds: old becomes consumed, new is findable", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt, _ := newTestRefreshToken(t)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}
		newRT, _ := rotateTestRefreshToken(t, rt)

		if err := repo.Rotate(ctx, rt.TokenHash(), newRT); err != nil {
			t.Fatalf("Rotate() unexpected error: %v", err)
		}

		if _, err := repo.FindByTokenHash(ctx, newRT.TokenHash()); err != nil {
			t.Fatalf("FindByTokenHash() for the newly rotated token: unexpected error: %v", err)
		}
		if err := repo.Rotate(ctx, rt.TokenHash(), newRT); !errors.Is(err, refreshtoken.ErrReused) {
			t.Fatalf("second Rotate() of the now-consumed old token: error = %v, want wrapping %v", err, refreshtoken.ErrReused)
		}
	})

	t.Run("Rotate on an unknown hash returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		unknown := refreshtoken.HashToken("never-issued-plaintext")
		candidate, _ := newTestRefreshToken(t)

		if err := repo.Rotate(context.Background(), unknown, candidate); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Fatalf("Rotate() error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
		}
	})

	t.Run("Rotate on an expired token returns ErrNotFound (no live family to protect)", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt := newRefreshTokenExpiringAt(t, -1*time.Minute)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}
		newRT, _ := rotateTestRefreshToken(t, rt)

		if err := repo.Rotate(ctx, rt.TokenHash(), newRT); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Fatalf("Rotate() on an expired token: error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
		}
	})

	t.Run("Rotate on an already-consumed token returns ErrReused, not ErrNotFound (reuse detection precedence)", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt, _ := newTestRefreshToken(t)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}
		rotated1, _ := rotateTestRefreshToken(t, rt)
		if err := repo.Rotate(ctx, rt.TokenHash(), rotated1); err != nil {
			t.Fatalf("setup first Rotate() unexpected error: %v", err)
		}
		// A second, distinct candidate for the same (now-consumed)
		// oldHash: the outcome must depend only on oldHash's state, not
		// on which candidate is proposed.
		rotated2, _ := rotateTestRefreshToken(t, rt)

		err := repo.Rotate(ctx, rt.TokenHash(), rotated2)
		if !errors.Is(err, refreshtoken.ErrReused) {
			t.Fatalf("Rotate() on an already-consumed token: error = %v, want wrapping %v", err, refreshtoken.ErrReused)
		}
	})

	t.Run("RevokeFamily deletes every token in the family, and only that family", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		rt, _ := newTestRefreshToken(t)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}
		rotated, _ := rotateTestRefreshToken(t, rt)
		if err := repo.Rotate(ctx, rt.TokenHash(), rotated); err != nil {
			t.Fatalf("setup Rotate() unexpected error: %v", err)
		}

		other, _ := newTestRefreshToken(t) // a distinct family, must be unaffected
		if err := repo.Save(ctx, other); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if err := repo.RevokeFamily(ctx, rt.FamilyID()); err != nil {
			t.Fatalf("RevokeFamily() unexpected error: %v", err)
		}

		if _, err := repo.FindByTokenHash(ctx, rotated.TokenHash()); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Errorf("FindByTokenHash() for the revoked family's active token: error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
		}
		if _, err := repo.FindByTokenHash(ctx, rt.TokenHash()); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Errorf("FindByTokenHash() for the revoked family's consumed token: error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
		}
		if _, err := repo.FindByTokenHash(ctx, other.TokenHash()); err != nil {
			t.Errorf("FindByTokenHash() for a different family's token after RevokeFamily: unexpected error: %v, want nil (other families must be unaffected)", err)
		}
	})

	t.Run("RevokeFamily on a family with no rows is not an error (idempotent)", func(t *testing.T) {
		repo := newRepo(t)
		rt, _ := newTestRefreshToken(t)

		if err := repo.RevokeFamily(context.Background(), rt.FamilyID()); err != nil {
			t.Fatalf("RevokeFamily() on an unknown family: unexpected error: %v, want nil", err)
		}
	})

	t.Run("TTL boundary: a token that has not yet expired is still found and rotatable", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		// Expires 2s in the future: comfortably larger than any
		// Save->FindByTokenHash/Rotate round-trip latency (including a
		// real Postgres round trip), while still exercising the "not yet
		// past expiresAt" edge rather than the full 30-day TTL.
		rt := newRefreshTokenExpiringAt(t, 2*time.Second)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if _, err := repo.FindByTokenHash(ctx, rt.TokenHash()); err != nil {
			t.Fatalf("FindByTokenHash() unexpected error: %v (token is still within its TTL)", err)
		}
		newRT, _ := rotateTestRefreshToken(t, rt)
		if err := repo.Rotate(ctx, rt.TokenHash(), newRT); err != nil {
			t.Fatalf("Rotate() unexpected error: %v (token is still within its TTL)", err)
		}
	})

	t.Run("TTL boundary: a token that has already expired is neither found nor rotatable", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt := newRefreshTokenExpiringAt(t, -2*time.Second)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if _, err := repo.FindByTokenHash(ctx, rt.TokenHash()); !errors.Is(err, refreshtoken.ErrNotFound) {
			t.Errorf("FindByTokenHash() error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
		}
	})

	t.Run("Rotate is atomic under concurrency: exactly one racer succeeds (nil), every other observes ErrReused", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		rt, _ := newTestRefreshToken(t)
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		const n = 20
		// Every racer proposes its own distinct candidate new token
		// (built synchronously, before any goroutine starts, since
		// t.Fatalf/t.Helper must run on the test's own goroutine): the
		// contract is about oldHash's state, not about the candidates
		// being identical.
		candidates := make([]*refreshtoken.RefreshToken, n)
		for i := range candidates {
			candidates[i], _ = rotateTestRefreshToken(t, rt)
		}

		var successCount, reusedCount, otherErrCount int64
		var wg sync.WaitGroup
		wg.Add(n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			candidate := candidates[i]
			go func() {
				defer wg.Done()
				<-start // every goroutine begins racing at (as close as possible to) the same instant
				err := repo.Rotate(ctx, rt.TokenHash(), candidate)
				switch {
				case err == nil:
					atomic.AddInt64(&successCount, 1)
				case errors.Is(err, refreshtoken.ErrReused):
					atomic.AddInt64(&reusedCount, 1)
				default:
					atomic.AddInt64(&otherErrCount, 1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if successCount != 1 {
			t.Errorf("successCount = %d, want exactly 1 (rotation must be atomic under concurrent Rotate calls)", successCount)
		}
		if reusedCount != n-1 {
			t.Errorf("reusedCount = %d, want %d", reusedCount, n-1)
		}
		if otherErrCount != 0 {
			t.Errorf("otherErrCount = %d, want 0 (every losing caller must observe exactly ErrReused, nothing else)", otherErrCount)
		}
	})
}

// newTestRefreshToken issues a fresh RefreshToken (expiresAt =
// time.Now() + refreshtoken.RefreshTokenTTL, via refreshtoken.Issue)
// bound to fixed test client/user/scope values. Each call returns an
// independent instance (fresh random Token, TokenHash and FamilyID).
func newTestRefreshToken(t *testing.T) (*refreshtoken.RefreshToken, refreshtoken.Token) {
	t.Helper()

	scope, err := refreshtoken.ParseScope("openid profile")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}
	rt, plaintext, err := refreshtoken.Issue(
		refreshtoken.NewClientID("client-1"),
		refreshtoken.NewUserID("user-1"),
		scope,
	)
	if err != nil {
		t.Fatalf("setup Issue() unexpected error: %v", err)
	}
	return rt, plaintext
}

// rotateTestRefreshToken wraps rt.Rotate(rt.Scope()), failing the test
// (via t.Fatalf) on error. It is used to build a *new* candidate
// RefreshToken in rt's family for repository-level Save/Rotate setup;
// the domain-level Rotate call itself does not mutate rt (only
// Repository.Rotate is the atomic, authoritative state transition),
// so this is safe to call multiple times against the same rt to
// produce several distinct candidates.
func rotateTestRefreshToken(t *testing.T, rt *refreshtoken.RefreshToken) (*refreshtoken.RefreshToken, refreshtoken.Token) {
	t.Helper()
	newRT, plaintext, err := rt.Rotate(rt.Scope())
	if err != nil {
		t.Fatalf("setup Rotate() unexpected error: %v", err)
	}
	return newRT, plaintext
}

// newRefreshTokenExpiringAt returns a RefreshToken identical to one
// built by newTestRefreshToken, except its expiresAt is fixed to
// time.Now().Add(offset) via refreshtoken.Reconstruct -- never via
// sleeping. A negative offset produces an already-expired fixture; a
// positive one produces one that expires imminently but has not yet.
func newRefreshTokenExpiringAt(t *testing.T, offset time.Duration) *refreshtoken.RefreshToken {
	t.Helper()
	base, _ := newTestRefreshToken(t)
	return refreshtoken.Reconstruct(
		base.TokenHash(), base.FamilyID(), base.ClientID(), base.UserID(), base.Scope(),
		time.Now().Add(offset), false,
	)
}

// assertSameRefreshToken compares every observable field of got
// against want.
//
// ExpiresAt is compared with microsecond truncation: Postgres's
// timestamptz column has microsecond precision, while Go's
// time.Now() carries nanoseconds, so an exact time.Time == comparison
// would spuriously fail once this contract is exercised against a
// real database (mirrors repotest.assertSameAuthCode).
func assertSameRefreshToken(t *testing.T, got, want *refreshtoken.RefreshToken) {
	t.Helper()
	if got.TokenHash() != want.TokenHash() {
		t.Errorf("TokenHash() = %v, want %v", got.TokenHash(), want.TokenHash())
	}
	if got.FamilyID() != want.FamilyID() {
		t.Errorf("FamilyID() = %v, want %v", got.FamilyID(), want.FamilyID())
	}
	if got.ClientID() != want.ClientID() {
		t.Errorf("ClientID() = %v, want %v", got.ClientID(), want.ClientID())
	}
	if got.UserID() != want.UserID() {
		t.Errorf("UserID() = %v, want %v", got.UserID(), want.UserID())
	}
	if got.Scope().String() != want.Scope().String() {
		t.Errorf("Scope() = %v, want %v", got.Scope(), want.Scope())
	}
	if got.Consumed() != want.Consumed() {
		t.Errorf("Consumed() = %v, want %v", got.Consumed(), want.Consumed())
	}
	if !got.ExpiresAt().Truncate(time.Microsecond).Equal(want.ExpiresAt().Truncate(time.Microsecond)) {
		t.Errorf("ExpiresAt() = %v, want %v", got.ExpiresAt(), want.ExpiresAt())
	}
}
