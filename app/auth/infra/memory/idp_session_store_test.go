// Regression tests for ISSUE-044 (the IdP session in-memory store had
// no background purge, so pending /authorize requests abandoned before
// login completes accumulated without bound). PurgeExpired is exercised
// via the public API only: expired entries are created deterministically
// by passing a negative ttl to CreateSession/SavePendingAuthorize (their
// ExpiresAt is now.Add(ttl), so a negative ttl produces an
// already-expired entry immediately, with no sleep required -- the same
// technique infra/postgres/purge_integration_test.go uses via raw SQL
// insert for the Postgres-backed aggregates).
package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/idpsession"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
)

func testUserID(t *testing.T) user.UserID {
	t.Helper()
	id, err := user.ParseUserID("user-1")
	if err != nil {
		t.Fatalf("setup ParseUserID() unexpected error: %v", err)
	}
	return id
}

// TestIdPSessionStore_PurgeExpired_RemovesOnlyExpiredEntries covers the
// core purge behavior: PurgeExpired deletes exactly the expired
// sessions/pending entries (reporting their count) and leaves live
// entries both present and still findable.
func TestIdPSessionStore_PurgeExpired_RemovesOnlyExpiredEntries(t *testing.T) {
	store := memory.NewIdPSessionStore()
	ctx := context.Background()
	uid := testUserID(t)

	// Live entries (positive ttl): must survive the purge.
	liveSession, err := store.CreateSession(ctx, uid, 1*time.Hour)
	if err != nil {
		t.Fatalf("setup CreateSession(live) unexpected error: %v", err)
	}
	livePending, err := store.SavePendingAuthorize(ctx, "response_type=code", 10*time.Minute)
	if err != nil {
		t.Fatalf("setup SavePendingAuthorize(live) unexpected error: %v", err)
	}

	// Expired entries (negative ttl -> ExpiresAt already in the past):
	// simulates a session past its 24h TTL and an /authorize request
	// abandoned before login completed (ISSUE-044's OOM scenario).
	expiredSession, err := store.CreateSession(ctx, uid, -1*time.Hour)
	if err != nil {
		t.Fatalf("setup CreateSession(expired) unexpected error: %v", err)
	}
	expiredPending, err := store.SavePendingAuthorize(ctx, "response_type=code", -10*time.Minute)
	if err != nil {
		t.Fatalf("setup SavePendingAuthorize(expired) unexpected error: %v", err)
	}

	purged, err := store.PurgeExpired(ctx)
	if err != nil {
		t.Fatalf("PurgeExpired() unexpected error: %v", err)
	}
	if purged != 2 {
		t.Errorf("PurgeExpired() purged = %d, want 2 (1 expired session + 1 expired pending)", purged)
	}

	// Live entries remain findable.
	if _, err := store.FindSession(ctx, liveSession.ID); err != nil {
		t.Errorf("FindSession(live) after purge: %v, want nil", err)
	}
	if _, err := store.FindPendingAuthorize(ctx, livePending.ID); err != nil {
		t.Errorf("FindPendingAuthorize(live) after purge: %v, want nil", err)
	}

	// Expired entries are gone.
	if _, err := store.FindSession(ctx, expiredSession.ID); err == nil {
		t.Error("FindSession(expired) after purge: got nil error, want idpsession.ErrNotFound")
	} else if !errors.Is(err, idpsession.ErrNotFound) {
		t.Errorf("FindSession(expired) after purge: error = %v, want wrapping %v", err, idpsession.ErrNotFound)
	}
	if _, err := store.FindPendingAuthorize(ctx, expiredPending.ID); err == nil {
		t.Error("FindPendingAuthorize(expired) after purge: got nil error, want idpsession.ErrNotFound")
	} else if !errors.Is(err, idpsession.ErrNotFound) {
		t.Errorf("FindPendingAuthorize(expired) after purge: error = %v, want wrapping %v", err, idpsession.ErrNotFound)
	}
}

// TestIdPSessionStore_PurgeExpired_NoOpWhenNothingExpired covers the
// boundary case: an empty/all-live store purges zero entries and
// leaves live entries untouched.
func TestIdPSessionStore_PurgeExpired_NoOpWhenNothingExpired(t *testing.T) {
	store := memory.NewIdPSessionStore()
	ctx := context.Background()
	uid := testUserID(t)

	sess, err := store.CreateSession(ctx, uid, 1*time.Hour)
	if err != nil {
		t.Fatalf("setup CreateSession() unexpected error: %v", err)
	}

	purged, err := store.PurgeExpired(ctx)
	if err != nil {
		t.Fatalf("PurgeExpired() unexpected error: %v", err)
	}
	if purged != 0 {
		t.Errorf("PurgeExpired() purged = %d, want 0", purged)
	}

	if _, err := store.FindSession(ctx, sess.ID); err != nil {
		t.Errorf("FindSession() after no-op purge: %v, want nil", err)
	}
}

// TestIdPSessionStore_PurgeExpired_CanceledContext covers the context
// cancellation guard shared with every other method on IdPSessionStore.
func TestIdPSessionStore_PurgeExpired_CanceledContext(t *testing.T) {
	store := memory.NewIdPSessionStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.PurgeExpired(ctx); err == nil {
		t.Error("PurgeExpired(canceled ctx) got nil error, want context.Canceled")
	} else if !errors.Is(err, context.Canceled) {
		t.Errorf("PurgeExpired(canceled ctx) error = %v, want wrapping %v", err, context.Canceled)
	}
}
