package authcode_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

// testCodeVerifier is a valid (43-character, unreserved-charset)
// code_verifier per RFC 7636 4.1. It is built with strings.Repeat
// rather than a hand-counted literal to avoid off-by-one length bugs.
var testCodeVerifier = strings.Repeat("A", 43)

func testChallenge(t *testing.T, verifier string) authcode.CodeChallenge {
	t.Helper()
	cc, err := authcode.NewCodeChallenge(s256Challenge(verifier), authcode.CodeChallengeMethodS256)
	if err != nil {
		t.Fatalf("setup NewCodeChallenge() unexpected error: %v", err)
	}
	return cc
}

// newValidAuthCode builds a freshly issued, unconsumed, non-expired
// AuthorizationCode bound to fixed test client/user/redirect/PKCE
// values. Each call returns an independent instance so table-driven
// subtests that mutate state (Consume) never leak into one another.
func newValidAuthCode(t *testing.T) *authcode.AuthorizationCode {
	t.Helper()

	scope, err := authcode.ParseScope("openid profile")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}

	ac, err := authcode.New(
		authcode.NewClientID("client-1"),
		authcode.NewUserID("user-1"),
		authcode.NewRedirectURI("http://localhost/callback"),
		scope,
		authcode.NewNonce("nonce-1"),
		testChallenge(t, testCodeVerifier),
		time.Time{},
	)
	if err != nil {
		t.Fatalf("setup New() unexpected error: %v", err)
	}
	return ac
}

func TestNew(t *testing.T) {
	ac := newValidAuthCode(t)

	if ac.Code().String() == "" {
		t.Error("Code().String() is empty, want non-empty opaque value")
	}
	if ac.Consumed() {
		t.Error("Consumed() = true for a freshly issued code, want false")
	}
	if ac.IsExpired() {
		t.Error("IsExpired() = true for a freshly issued code, want false")
	}
	if ac.ClientID() != authcode.NewClientID("client-1") {
		t.Errorf("ClientID() = %v, want %v", ac.ClientID(), authcode.NewClientID("client-1"))
	}
	if ac.UserID() != authcode.NewUserID("user-1") {
		t.Errorf("UserID() = %v, want %v", ac.UserID(), authcode.NewUserID("user-1"))
	}
	if ac.RedirectURI() != authcode.NewRedirectURI("http://localhost/callback") {
		t.Errorf("RedirectURI() = %v, want %v", ac.RedirectURI(), authcode.NewRedirectURI("http://localhost/callback"))
	}
	if ac.Nonce().String() != "nonce-1" {
		t.Errorf("Nonce().String() = %q, want %q", ac.Nonce().String(), "nonce-1")
	}

	// Two independently generated codes must not collide.
	other := newValidAuthCode(t)
	if ac.Code() == other.Code() {
		t.Error("two New() calls produced the same opaque Code, want distinct random values")
	}
}

func TestAuthorizationCode_Verify(t *testing.T) {
	validRedirect := authcode.NewRedirectURI("http://localhost/callback")
	validClient := authcode.NewClientID("client-1")

	t.Run("valid verify succeeds", func(t *testing.T) {
		ac := newValidAuthCode(t)

		if err := ac.Verify(testCodeVerifier, validRedirect, validClient); err != nil {
			t.Fatalf("Verify() unexpected error: %v", err)
		}
	})

	t.Run("redirect_uri mismatch is rejected", func(t *testing.T) {
		ac := newValidAuthCode(t)

		err := ac.Verify(testCodeVerifier, authcode.NewRedirectURI("http://localhost/other"), validClient)
		if !errors.Is(err, authcode.ErrRedirectURIMismatch) {
			t.Fatalf("Verify() error = %v, want wrapping %v", err, authcode.ErrRedirectURIMismatch)
		}
	})

	t.Run("client_id mismatch is rejected", func(t *testing.T) {
		ac := newValidAuthCode(t)

		err := ac.Verify(testCodeVerifier, validRedirect, authcode.NewClientID("other-client"))
		if !errors.Is(err, authcode.ErrClientMismatch) {
			t.Fatalf("Verify() error = %v, want wrapping %v", err, authcode.ErrClientMismatch)
		}
	})

	t.Run("PKCE code_verifier mismatch is rejected", func(t *testing.T) {
		ac := newValidAuthCode(t)

		err := ac.Verify(strings.Repeat("B", 43), validRedirect, validClient)
		if !errors.Is(err, authcode.ErrPKCEVerificationFailed) {
			t.Fatalf("Verify() error = %v, want wrapping %v", err, authcode.ErrPKCEVerificationFailed)
		}
	})

	t.Run("already consumed code is rejected", func(t *testing.T) {
		ac := newValidAuthCode(t)
		if err := ac.Consume(); err != nil {
			t.Fatalf("setup Consume() unexpected error: %v", err)
		}

		err := ac.Verify(testCodeVerifier, validRedirect, validClient)
		if !errors.Is(err, authcode.ErrAlreadyConsumed) {
			t.Fatalf("Verify() error = %v, want wrapping %v", err, authcode.ErrAlreadyConsumed)
		}
	})

	t.Run("expired code is rejected", func(t *testing.T) {
		ac := newValidAuthCode(t)
		expired := authcode.Reconstruct(
			ac.Code(), ac.ClientID(), ac.UserID(), ac.RedirectURI(),
			ac.Scope(), ac.Nonce(), ac.Challenge(),
			time.Time{}, time.Now().Add(-1*time.Minute), false,
		)

		err := expired.Verify(testCodeVerifier, validRedirect, validClient)
		if !errors.Is(err, authcode.ErrExpired) {
			t.Fatalf("Verify() error = %v, want wrapping %v", err, authcode.ErrExpired)
		}
	})
}

func TestAuthorizationCode_Consume(t *testing.T) {
	t.Run("first consume succeeds and marks consumed", func(t *testing.T) {
		ac := newValidAuthCode(t)

		if err := ac.Consume(); err != nil {
			t.Fatalf("Consume() unexpected error: %v", err)
		}
		if !ac.Consumed() {
			t.Error("Consumed() = false after Consume(), want true")
		}
	})

	t.Run("second consume fails (single-use)", func(t *testing.T) {
		ac := newValidAuthCode(t)
		if err := ac.Consume(); err != nil {
			t.Fatalf("setup Consume() unexpected error: %v", err)
		}

		err := ac.Consume()
		if !errors.Is(err, authcode.ErrAlreadyConsumed) {
			t.Fatalf("second Consume() error = %v, want wrapping %v", err, authcode.ErrAlreadyConsumed)
		}
	})
}

// TestAuthorizationCode_IsExpired injects expiresAt via Reconstruct
// (never via real-time sleep) to cover the far future/past cases
// (正/異) and the "just" past/future and "now" boundaries (境界).
func TestAuthorizationCode_IsExpired(t *testing.T) {
	base := newValidAuthCode(t)

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
			ac := authcode.Reconstruct(
				base.Code(), base.ClientID(), base.UserID(), base.RedirectURI(),
				base.Scope(), base.Nonce(), base.Challenge(),
				time.Time{}, tt.expiresAt, false,
			)

			if got := ac.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v (expiresAt=%v)", got, tt.want, tt.expiresAt)
			}
		})
	}

	t.Run("expiresAt set to exactly now is expired (evaluation always occurs strictly after)", func(t *testing.T) {
		now := time.Now()
		ac := authcode.Reconstruct(
			base.Code(), base.ClientID(), base.UserID(), base.RedirectURI(),
			base.Scope(), base.Nonce(), base.Challenge(),
			time.Time{}, now, false,
		)

		if !ac.IsExpired() {
			t.Error("IsExpired() = false for expiresAt == now, want true (time.Now() inside IsExpired is always later)")
		}
	})
}
