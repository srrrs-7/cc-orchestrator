package refreshtoken_test

import (
	"errors"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// testScope is a small helper wrapping refreshtoken.ParseScope for
// setup, matching domain/authcode's *_test.go convention of failing
// fast (t.Fatalf) on unexpected setup errors so table-driven
// assertions stay focused on the behavior under test.
func testScope(t *testing.T, s string) refreshtoken.Scope {
	t.Helper()
	scope, err := refreshtoken.ParseScope(s)
	if err != nil {
		t.Fatalf("setup ParseScope(%q) unexpected error: %v", s, err)
	}
	return scope
}

// newTestRefreshToken issues a brand new RefreshToken (via
// refreshtoken.Issue) bound to fixed test client/user/scope values.
// Each call returns an independent instance (fresh random Token,
// TokenHash and FamilyID) so table-driven subtests never leak state
// into one another.
func newTestRefreshToken(t *testing.T) (*refreshtoken.RefreshToken, refreshtoken.Token) {
	t.Helper()
	rt, plaintext, err := refreshtoken.Issue(
		refreshtoken.NewClientID("client-1"),
		refreshtoken.NewUserID("user-1"),
		testScope(t, "openid profile"),
	)
	if err != nil {
		t.Fatalf("setup Issue() unexpected error: %v", err)
	}
	return rt, plaintext
}

// TestIssue covers R2 (issuance at authorization_code exchange) and
// R8 (the persisted aggregate only ever exposes/derives a hash; the
// plaintext Token is a separate, one-time-use return value).
func TestIssue(t *testing.T) {
	rt, plaintext := newTestRefreshToken(t)

	if plaintext.String() == "" {
		t.Error("plaintext Token.String() is empty, want non-empty opaque value")
	}
	if rt.TokenHash() != plaintext.Hash() {
		t.Errorf("TokenHash() = %v, want %v (Token.Hash() of the plaintext Issue() returned)", rt.TokenHash(), plaintext.Hash())
	}
	if rt.Consumed() {
		t.Error("Consumed() = true for a freshly issued refresh token, want false")
	}
	if rt.ClientID() != refreshtoken.NewClientID("client-1") {
		t.Errorf("ClientID() = %v, want %v", rt.ClientID(), refreshtoken.NewClientID("client-1"))
	}
	if rt.UserID() != refreshtoken.NewUserID("user-1") {
		t.Errorf("UserID() = %v, want %v", rt.UserID(), refreshtoken.NewUserID("user-1"))
	}
	if !rt.Scope().Has("profile") {
		t.Error("Scope().Has(\"profile\") = false, want true")
	}

	wantExpiresAt := time.Now().Add(refreshtoken.RefreshTokenTTL)
	if diff := rt.ExpiresAt().Sub(wantExpiresAt); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt() = %v, want approximately %v (now + RefreshTokenTTL)", rt.ExpiresAt(), wantExpiresAt)
	}

	// Two independently issued tokens must never collide, in either
	// their opaque plaintext, their derived hash, or their family.
	other, otherPlaintext, err := refreshtoken.Issue(
		refreshtoken.NewClientID("client-1"),
		refreshtoken.NewUserID("user-1"),
		testScope(t, "openid profile"),
	)
	if err != nil {
		t.Fatalf("setup second Issue() unexpected error: %v", err)
	}
	if rt.TokenHash() == other.TokenHash() {
		t.Error("two Issue() calls produced the same TokenHash, want cryptographically distinct random values")
	}
	if rt.FamilyID() == other.FamilyID() {
		t.Error("two Issue() calls produced the same FamilyID, want cryptographically distinct random values")
	}
	if plaintext.String() == otherPlaintext.String() {
		t.Error("two Issue() calls produced the same plaintext Token, want cryptographically distinct random values")
	}
}

// TestRefreshToken_Rotate covers R4 (rotation: new Token/hash, same
// family, sliding expiry) and R7 (the caller-supplied scope becomes
// the new token's effective scope).
func TestRefreshToken_Rotate(t *testing.T) {
	rt, plaintext := newTestRefreshToken(t)
	newScope := testScope(t, "openid")

	newRT, newPlaintext, err := rt.Rotate(newScope)
	if err != nil {
		t.Fatalf("Rotate() unexpected error: %v", err)
	}

	if newRT.FamilyID() != rt.FamilyID() {
		t.Errorf("FamilyID() = %v, want %v (rotation must stay within the same family)", newRT.FamilyID(), rt.FamilyID())
	}
	if newRT.TokenHash() == rt.TokenHash() {
		t.Error("Rotate() produced the same TokenHash as the original, want a new one")
	}
	if newPlaintext.String() == plaintext.String() {
		t.Error("Rotate() produced the same plaintext Token as the original, want a new one")
	}
	if newRT.Consumed() {
		t.Error("Consumed() = true for a freshly rotated refresh token, want false")
	}
	if newRT.ClientID() != rt.ClientID() {
		t.Errorf("ClientID() = %v, want %v (unchanged across rotation)", newRT.ClientID(), rt.ClientID())
	}
	if newRT.UserID() != rt.UserID() {
		t.Errorf("UserID() = %v, want %v (unchanged across rotation)", newRT.UserID(), rt.UserID())
	}
	if newRT.Scope().String() != newScope.String() {
		t.Errorf("Scope() = %v, want the scope passed to Rotate() (%v)", newRT.Scope(), newScope)
	}

	// Sliding TTL: Rotate computes a fresh now+RefreshTokenTTL, which
	// (rotation necessarily happening strictly after issuance) must
	// not be earlier than the original's expiresAt.
	if newRT.ExpiresAt().Before(rt.ExpiresAt()) {
		t.Errorf("ExpiresAt() = %v, want not earlier than the original's %v (sliding TTL)", newRT.ExpiresAt(), rt.ExpiresAt())
	}
}

// TestRefreshToken_MatchesClient covers R6 (client binding).
func TestRefreshToken_MatchesClient(t *testing.T) {
	rt, _ := newTestRefreshToken(t)

	t.Run("matching client succeeds", func(t *testing.T) {
		if err := rt.MatchesClient(refreshtoken.NewClientID("client-1")); err != nil {
			t.Fatalf("MatchesClient() unexpected error: %v", err)
		}
	})

	t.Run("mismatched client is rejected", func(t *testing.T) {
		err := rt.MatchesClient(refreshtoken.NewClientID("other-client"))
		if !errors.Is(err, refreshtoken.ErrClientMismatch) {
			t.Fatalf("MatchesClient() error = %v, want wrapping %v", err, refreshtoken.ErrClientMismatch)
		}
	})
}

// TestRefreshToken_IsExpired injects expiresAt via Reconstruct (never
// via real-time sleep, per testing.md "実時間 sleep に依存するテストを書か
// ない"), covering far future/past (正/異) and the "just" past/future
// boundaries (境界), following
// domain/authcode/authorization_code_test.go's
// TestAuthorizationCode_IsExpired pattern.
func TestRefreshToken_IsExpired(t *testing.T) {
	base, _ := newTestRefreshToken(t)

	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "far in the future is not expired", expiresAt: time.Now().Add(1 * time.Hour), want: false},
		{name: "just in the future is not expired (boundary)", expiresAt: time.Now().Add(2 * time.Second), want: false},
		{name: "just in the past is expired (boundary)", expiresAt: time.Now().Add(-2 * time.Second), want: true},
		{name: "far in the past is expired", expiresAt: time.Now().Add(-1 * time.Hour), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := refreshtoken.Reconstruct(
				base.TokenHash(), base.FamilyID(), base.ClientID(), base.UserID(), base.Scope(),
				tt.expiresAt, false,
			)
			if got := rt.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v (expiresAt=%v)", got, tt.want, tt.expiresAt)
			}
		})
	}
}

// TestReconstruct_Getters verifies that every getter reflects exactly
// the state Reconstruct was given, including Consumed()=true -- a
// state Issue/Rotate never directly produce but infra/memory and
// infra/postgres must be able to round-trip via this constructor when
// loading a consumed-but-unexpired row (needed for reuse detection,
// see repotest.RunRefreshTokenRepositoryContract).
func TestReconstruct_Getters(t *testing.T) {
	base, _ := newTestRefreshToken(t)
	expiresAt := time.Now().Add(1 * time.Hour)

	rt := refreshtoken.Reconstruct(base.TokenHash(), base.FamilyID(), base.ClientID(), base.UserID(), base.Scope(), expiresAt, true)

	if rt.TokenHash() != base.TokenHash() {
		t.Errorf("TokenHash() = %v, want %v", rt.TokenHash(), base.TokenHash())
	}
	if rt.FamilyID() != base.FamilyID() {
		t.Errorf("FamilyID() = %v, want %v", rt.FamilyID(), base.FamilyID())
	}
	if rt.ClientID() != base.ClientID() {
		t.Errorf("ClientID() = %v, want %v", rt.ClientID(), base.ClientID())
	}
	if rt.UserID() != base.UserID() {
		t.Errorf("UserID() = %v, want %v", rt.UserID(), base.UserID())
	}
	if rt.Scope().String() != base.Scope().String() {
		t.Errorf("Scope() = %v, want %v", rt.Scope(), base.Scope())
	}
	if !rt.ExpiresAt().Equal(expiresAt) {
		t.Errorf("ExpiresAt() = %v, want %v", rt.ExpiresAt(), expiresAt)
	}
	if !rt.Consumed() {
		t.Error("Consumed() = false, want true (as passed to Reconstruct)")
	}
}
