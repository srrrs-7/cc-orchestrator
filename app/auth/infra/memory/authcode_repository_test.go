package memory_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
)

// newExpiredAuthCode builds an AuthorizationCode that is bit-for-bit
// identical to base except its expiresAt is in the past, via
// Reconstruct (never via sleep).
func newExpiredAuthCode(base *authcode.AuthorizationCode) *authcode.AuthorizationCode {
	return authcode.Reconstruct(
		base.Code(), base.ClientID(), base.UserID(), base.RedirectURI(),
		base.Scope(), base.Nonce(), base.Challenge(),
		time.Now().Add(-1*time.Minute), false,
	)
}

func newAuthCode(t *testing.T) *authcode.AuthorizationCode {
	t.Helper()

	scope, err := authcode.ParseScope("openid")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}
	challenge, err := authcode.NewCodeChallenge("some-challenge-value", authcode.CodeChallengeMethodS256)
	if err != nil {
		t.Fatalf("setup NewCodeChallenge() unexpected error: %v", err)
	}

	ac, err := authcode.New(
		authcode.NewClientID("client-1"),
		authcode.NewUserID("user-1"),
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

func TestAuthCodeRepository_SaveAndFindByCode(t *testing.T) {
	t.Run("saved code is found", func(t *testing.T) {
		repo := memory.NewAuthCodeRepository()
		ac := newAuthCode(t)

		if err := repo.Save(context.Background(), ac); err != nil {
			t.Fatalf("Save() unexpected error: %v", err)
		}

		got, err := repo.FindByCode(context.Background(), ac.Code())
		if err != nil {
			t.Fatalf("FindByCode() unexpected error: %v", err)
		}
		if got.Code() != ac.Code() {
			t.Errorf("Code() = %v, want %v", got.Code(), ac.Code())
		}
		if got.ClientID() != ac.ClientID() {
			t.Errorf("ClientID() = %v, want %v", got.ClientID(), ac.ClientID())
		}
		if got.Consumed() {
			t.Error("Consumed() = true for a freshly saved code, want false")
		}
	})

	t.Run("unsaved code is not found", func(t *testing.T) {
		repo := memory.NewAuthCodeRepository()

		unknownCode, err := authcode.ParseCode("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseCode() unexpected error: %v", err)
		}

		_, err = repo.FindByCode(context.Background(), unknownCode)
		if !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("FindByCode() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
	})

	t.Run("re-saving after Consume persists the consumed state", func(t *testing.T) {
		repo := memory.NewAuthCodeRepository()
		ac := newAuthCode(t)
		if err := repo.Save(context.Background(), ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if err := ac.Consume(); err != nil {
			t.Fatalf("setup Consume() unexpected error: %v", err)
		}
		if err := repo.Save(context.Background(), ac); err != nil {
			t.Fatalf("Save() after Consume() unexpected error: %v", err)
		}

		got, err := repo.FindByCode(context.Background(), ac.Code())
		if err != nil {
			t.Fatalf("FindByCode() unexpected error: %v", err)
		}
		if !got.Consumed() {
			t.Error("Consumed() = false after re-Save() of a consumed code, want true")
		}
	})
}

// TestAuthCodeRepository_CloneIndependence verifies that mutating a
// *authcode.AuthorizationCode obtained from FindByCode (e.g. calling
// Consume on it) without an explicit Save does not leak into the
// repository's stored state: the repository must hand out clones, not
// aliases to its internal data.
func TestAuthCodeRepository_CloneIndependence(t *testing.T) {
	repo := memory.NewAuthCodeRepository()
	ac := newAuthCode(t)
	if err := repo.Save(context.Background(), ac); err != nil {
		t.Fatalf("setup Save() unexpected error: %v", err)
	}

	got1, err := repo.FindByCode(context.Background(), ac.Code())
	if err != nil {
		t.Fatalf("FindByCode() unexpected error: %v", err)
	}
	if err := got1.Consume(); err != nil {
		t.Fatalf("Consume() on the retrieved clone unexpected error: %v", err)
	}
	// Deliberately not calling repo.Save(ctx, got1) here: the mutation
	// above must stay local to got1.

	got2, err := repo.FindByCode(context.Background(), ac.Code())
	if err != nil {
		t.Fatalf("FindByCode() unexpected error: %v", err)
	}
	if got2.Consumed() {
		t.Error("Consumed() = true on a fresh FindByCode() after mutating an earlier clone without Save(), want false (clones must not alias the repository's stored data)")
	}
}

// TestAuthCodeRepository_Consume covers Repository.Consume's
// documented semantics: successful redemption deletes the entry
// (正), an unknown code and a repeat Consume of an already-redeemed
// code both report ErrNotFound rather than ErrAlreadyConsumed (異,
// delete-based single-use), and an expired-but-present entry reports
// ErrExpired while still being evicted (境界: existing vs. expired).
func TestAuthCodeRepository_Consume(t *testing.T) {
	t.Run("consuming an existing, non-expired code succeeds and deletes it", func(t *testing.T) {
		repo := memory.NewAuthCodeRepository()
		ac := newAuthCode(t)
		if err := repo.Save(context.Background(), ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if err := repo.Consume(context.Background(), ac.Code()); err != nil {
			t.Fatalf("Consume() unexpected error: %v", err)
		}

		if _, err := repo.FindByCode(context.Background(), ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("FindByCode() after Consume() error = %v, want wrapping %v (entry must be deleted on successful consume)", err, authcode.ErrNotFound)
		}
	})

	t.Run("consuming an unknown code fails with ErrNotFound", func(t *testing.T) {
		repo := memory.NewAuthCodeRepository()

		unknownCode, err := authcode.ParseCode("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseCode() unexpected error: %v", err)
		}

		err = repo.Consume(context.Background(), unknownCode)
		if !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("Consume() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
	})

	t.Run("consuming an already-consumed code fails with ErrNotFound, not ErrAlreadyConsumed", func(t *testing.T) {
		repo := memory.NewAuthCodeRepository()
		ac := newAuthCode(t)
		if err := repo.Save(context.Background(), ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}
		if err := repo.Consume(context.Background(), ac.Code()); err != nil {
			t.Fatalf("setup Consume() unexpected error: %v", err)
		}

		err := repo.Consume(context.Background(), ac.Code())
		if !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("second Consume() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
		if errors.Is(err, authcode.ErrAlreadyConsumed) {
			t.Error("second Consume() error wraps authcode.ErrAlreadyConsumed, want it not to: this delete-based repository never returns ErrAlreadyConsumed, a repeat looks identical to not-found")
		}
	})

	t.Run("consuming an expired code fails with ErrExpired and evicts it", func(t *testing.T) {
		repo := memory.NewAuthCodeRepository()
		ac := newAuthCode(t)
		expired := newExpiredAuthCode(ac)
		if err := repo.Save(context.Background(), expired); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		err := repo.Consume(context.Background(), ac.Code())
		if !errors.Is(err, authcode.ErrExpired) {
			t.Fatalf("Consume() error = %v, want wrapping %v", err, authcode.ErrExpired)
		}

		if _, err := repo.FindByCode(context.Background(), ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("FindByCode() after Consume() on an expired entry error = %v, want wrapping %v (entry must be evicted)", err, authcode.ErrNotFound)
		}
	})
}

// TestAuthCodeRepository_FindByCode_EvictsExpiredEntries covers the
// lazy-eviction contract: a lookup of an expired entry must report
// ErrNotFound *and* actually remove the entry, not merely mask it --
// verified by observing the same ErrNotFound on a second lookup with
// no intervening write.
func TestAuthCodeRepository_FindByCode_EvictsExpiredEntries(t *testing.T) {
	repo := memory.NewAuthCodeRepository()
	ac := newAuthCode(t)
	expired := newExpiredAuthCode(ac)
	if err := repo.Save(context.Background(), expired); err != nil {
		t.Fatalf("setup Save() unexpected error: %v", err)
	}

	if _, err := repo.FindByCode(context.Background(), ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
		t.Fatalf("first FindByCode() error = %v, want wrapping %v", err, authcode.ErrNotFound)
	}
	if _, err := repo.FindByCode(context.Background(), ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
		t.Fatalf("second FindByCode() error = %v, want wrapping %v (the entry must have been evicted by the first lookup)", err, authcode.ErrNotFound)
	}
}

// TestAuthCodeRepository_Consume_ConcurrentSingleUse is the
// most important test in this file: it seeds exactly one
// AuthorizationCode and fires n goroutines at Consume for the same
// code simultaneously (released together via a closed channel, never
// via sleep). Exactly one call may succeed; every other call --
// including every loser of the race -- must observe ErrNotFound and
// nothing else. Run with `go test -race` to confirm Consume's single
// mu.Lock() critical section has no data race.
func TestAuthCodeRepository_Consume_ConcurrentSingleUse(t *testing.T) {
	repo := memory.NewAuthCodeRepository()
	ac := newAuthCode(t)
	if err := repo.Save(context.Background(), ac); err != nil {
		t.Fatalf("setup Save() unexpected error: %v", err)
	}

	const n = 50
	var successCount, notFoundCount, otherErrCount int64

	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start // every goroutine begins racing at (as close as possible to) the same instant

			err := repo.Consume(context.Background(), ac.Code())
			switch {
			case err == nil:
				atomic.AddInt64(&successCount, 1)
			case errors.Is(err, authcode.ErrNotFound):
				atomic.AddInt64(&notFoundCount, 1)
			default:
				atomic.AddInt64(&otherErrCount, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if successCount != 1 {
		t.Errorf("successCount = %d, want exactly 1 (single-use must hold under concurrent Consume calls)", successCount)
	}
	if notFoundCount != n-1 {
		t.Errorf("notFoundCount = %d, want %d", notFoundCount, n-1)
	}
	if otherErrCount != 0 {
		t.Errorf("otherErrCount = %d, want 0 (every losing caller must observe exactly ErrNotFound, nothing else)", otherErrCount)
	}
}
