package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
)

// newRefreshToken issues a fresh refreshtoken.RefreshToken bound to
// fixed test client/user/scope values (mirrors
// repotest.newTestRefreshToken, duplicated here in package memory_test
// since repotest's helper is unexported).
func newRefreshToken(t *testing.T) *refreshtoken.RefreshToken {
	t.Helper()

	scope, err := refreshtoken.ParseScope("openid profile")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}
	rt, _, err := refreshtoken.Issue(
		refreshtoken.NewClientID("client-1"),
		refreshtoken.NewUserID("user-1"),
		scope,
	)
	if err != nil {
		t.Fatalf("setup Issue() unexpected error: %v", err)
	}
	return rt
}

// TestRefreshTokenRepository_FindByTokenHash_ReturnsIndependentClones
// verifies that FindByTokenHash never hands back the exact same
// *refreshtoken.RefreshToken pointer the repository holds internally
// (or a pointer previously returned to another caller): each call
// must return its own clone, so no caller can accidentally alias --
// and through that alias, observe or corrupt -- the repository's
// stored state (mirrors
// TestAuthCodeRepository_CloneIndependence's intent, adapted for
// RefreshToken's getter-only, no-mutator aggregate shape).
func TestRefreshTokenRepository_FindByTokenHash_ReturnsIndependentClones(t *testing.T) {
	repo := memory.NewRefreshTokenRepository()
	rt := newRefreshToken(t)
	if err := repo.Save(context.Background(), rt); err != nil {
		t.Fatalf("setup Save() unexpected error: %v", err)
	}

	got1, err := repo.FindByTokenHash(context.Background(), rt.TokenHash())
	if err != nil {
		t.Fatalf("first FindByTokenHash() unexpected error: %v", err)
	}
	got2, err := repo.FindByTokenHash(context.Background(), rt.TokenHash())
	if err != nil {
		t.Fatalf("second FindByTokenHash() unexpected error: %v", err)
	}

	if got1 == rt {
		t.Error("FindByTokenHash() returned the same pointer passed to Save(), want an independent clone")
	}
	if got1 == got2 {
		t.Error("two FindByTokenHash() calls returned the same pointer, want independent clones each time")
	}
	if got1.TokenHash() != got2.TokenHash() || got1.Consumed() != got2.Consumed() {
		t.Errorf("clones diverge on observable state: got1=%+v, got2 Consumed()=%v", got1, got2.Consumed())
	}
}

// TestRefreshTokenRepository_Rotate_OldEntryStaysConsumedInTheStore
// exercises Rotate's memory-specific mechanics directly (see
// Rotate's doc comment: the old entry is rebuilt via
// refreshtoken.Reconstruct(..., consumed=true) and the map entry
// replaced, rather than mutated in place): after a successful Rotate,
// the old token's map entry is still present (not deleted) and
// reports Consumed()=true, while the new token is reachable under its
// own, distinct hash.
func TestRefreshTokenRepository_Rotate_OldEntryStaysConsumedInTheStore(t *testing.T) {
	repo := memory.NewRefreshTokenRepository()
	ctx := context.Background()
	rt := newRefreshToken(t)
	if err := repo.Save(ctx, rt); err != nil {
		t.Fatalf("setup Save() unexpected error: %v", err)
	}

	newRT, _, err := rt.Rotate(rt.Scope())
	if err != nil {
		t.Fatalf("setup Rotate() unexpected error: %v", err)
	}
	if err := repo.Rotate(ctx, rt.TokenHash(), newRT); err != nil {
		t.Fatalf("Rotate() unexpected error: %v", err)
	}

	oldGot, err := repo.FindByTokenHash(ctx, rt.TokenHash())
	if err != nil {
		t.Fatalf("FindByTokenHash() for the old (now consumed) hash: unexpected error: %v", err)
	}
	if !oldGot.Consumed() {
		t.Error("old entry Consumed() = false after Rotate(), want true")
	}
	if oldGot.FamilyID() != rt.FamilyID() {
		t.Errorf("old entry FamilyID() = %v, want unchanged %v", oldGot.FamilyID(), rt.FamilyID())
	}

	newGot, err := repo.FindByTokenHash(ctx, newRT.TokenHash())
	if err != nil {
		t.Fatalf("FindByTokenHash() for the new hash: unexpected error: %v", err)
	}
	if newGot.Consumed() {
		t.Error("new entry Consumed() = true immediately after Rotate(), want false")
	}
	if newGot.TokenHash() == oldGot.TokenHash() {
		t.Error("new entry's TokenHash() equals the old one's, want distinct hashes")
	}
}

// TestRefreshTokenRepository_RevokeFamily_OnlyDeletesMatchingHashKeys
// verifies RevokeFamily's map-scan deletion targets exactly the rows
// whose FamilyID matches, leaving a distinct hash key belonging to a
// different family both present and findable.
func TestRefreshTokenRepository_RevokeFamily_OnlyDeletesMatchingHashKeys(t *testing.T) {
	repo := memory.NewRefreshTokenRepository()
	ctx := context.Background()

	target := newRefreshToken(t)
	if err := repo.Save(ctx, target); err != nil {
		t.Fatalf("setup Save(target) unexpected error: %v", err)
	}
	other := newRefreshToken(t) // Issue() mints a fresh, distinct FamilyID each call.
	if err := repo.Save(ctx, other); err != nil {
		t.Fatalf("setup Save(other) unexpected error: %v", err)
	}

	if err := repo.RevokeFamily(ctx, target.FamilyID()); err != nil {
		t.Fatalf("RevokeFamily() unexpected error: %v", err)
	}

	if _, err := repo.FindByTokenHash(ctx, target.TokenHash()); !errors.Is(err, refreshtoken.ErrNotFound) {
		t.Errorf("FindByTokenHash(target) after RevokeFamily() error = %v, want wrapping %v", err, refreshtoken.ErrNotFound)
	}
	if _, err := repo.FindByTokenHash(ctx, other.TokenHash()); err != nil {
		t.Errorf("FindByTokenHash(other) after RevokeFamily(target's family): unexpected error: %v, want nil", err)
	}
}
