package repotest

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

// NewAuthCodeRepository constructs an authcode.Repository backed by a
// clean, empty store, ready for a single subtest.
//
// Implementations MUST return a repository whose store is empty every
// time this is called (a fresh in-memory map for infra/memory; a
// truncated table for infra/postgres), so that
// RunAuthCodeRepositoryContract's subtests never observe data left
// behind by another subtest.
type NewAuthCodeRepository func(t *testing.T) authcode.Repository

// testCodeVerifierValue is a fixed, valid PKCE code_verifier (RFC
// 7636 4.1: 43-128 characters from the unreserved charset). Using a
// fixed value (rather than a fresh random one per subtest) keeps
// every fixture's expected code_challenge derivable and reproducible.
var testCodeVerifierValue = strings.Repeat("v", 43)

// RunAuthCodeRepositoryContract runs the behavioral contract shared
// by every authcode.Repository implementation (SPEC-005 R2): Save /
// FindByCode round-trip every field, single-use redemption
// (Consume), and TTL-based expiry (including lazy eviction and
// atomicity under concurrent Consume calls for the same code) all
// behave identically for infra/memory and infra/postgres.
//
// Real-time sleeps are never used to exercise TTL: every
// expiring/expired fixture is built via authcode.Reconstruct with an
// expiresAt computed once, up front, relative to time.Now() (see
// newAuthCodeExpiringAt), per testing.md "実時間 sleep に依存するテストを
// 書かない".
func RunAuthCodeRepositoryContract(t *testing.T, newRepo NewAuthCodeRepository) {
	t.Helper()

	t.Run("Save then FindByCode round-trips every field, Consumed()=false", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ac := newTestAuthCode(t, testCodeVerifierValue)

		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("Save() unexpected error: %v", err)
		}

		got, err := repo.FindByCode(ctx, ac.Code())
		if err != nil {
			t.Fatalf("FindByCode() unexpected error: %v", err)
		}
		assertSameAuthCode(t, got, ac, testCodeVerifierValue)
		if got.Consumed() {
			t.Error("Consumed() = true for a freshly saved code, want false")
		}
	})

	t.Run("FindByCode for a code that was never saved returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		unknown, err := authcode.ParseCode("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseCode() unexpected error: %v", err)
		}

		if _, err := repo.FindByCode(context.Background(), unknown); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("FindByCode() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
	})

	t.Run("FindByCode for an expired code returns ErrNotFound and evicts it (lazy deletion)", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ac := newAuthCodeExpiringAt(t, testCodeVerifierValue, -1*time.Minute)
		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if _, err := repo.FindByCode(ctx, ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("first FindByCode() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
		if _, err := repo.FindByCode(ctx, ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("second FindByCode() error = %v, want wrapping %v (entry must have been evicted by the first lookup)", err, authcode.ErrNotFound)
		}
	})

	t.Run("Consume on an existing, non-expired code succeeds and the code becomes unfindable", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ac := newTestAuthCode(t, testCodeVerifierValue)
		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if err := repo.Consume(ctx, ac.Code()); err != nil {
			t.Fatalf("Consume() unexpected error: %v", err)
		}
		if _, err := repo.FindByCode(ctx, ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("FindByCode() after Consume() error = %v, want wrapping %v (entry must be deleted on successful consume)", err, authcode.ErrNotFound)
		}
	})

	t.Run("Consume on an unknown code returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		unknown, err := authcode.ParseCode("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseCode() unexpected error: %v", err)
		}

		if err := repo.Consume(context.Background(), unknown); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("Consume() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
	})

	t.Run("Consume is single-use: a second Consume of an already-consumed code returns ErrNotFound, not ErrAlreadyConsumed", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ac := newTestAuthCode(t, testCodeVerifierValue)
		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}
		if err := repo.Consume(ctx, ac.Code()); err != nil {
			t.Fatalf("first Consume() unexpected error: %v", err)
		}

		err := repo.Consume(ctx, ac.Code())
		if !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("second Consume() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
		if errors.Is(err, authcode.ErrAlreadyConsumed) {
			t.Error("second Consume() error wraps authcode.ErrAlreadyConsumed, want it not to: a delete-based repository must make a repeat redemption look identical to not-found (see domain/authcode/repository.go)")
		}
	})

	t.Run("Consume on an expired code returns ErrExpired and evicts it", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ac := newAuthCodeExpiringAt(t, testCodeVerifierValue, -1*time.Minute)
		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		err := repo.Consume(ctx, ac.Code())
		if !errors.Is(err, authcode.ErrExpired) {
			t.Fatalf("Consume() error = %v, want wrapping %v", err, authcode.ErrExpired)
		}
		if _, err := repo.FindByCode(ctx, ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("FindByCode() after Consume() on an expired entry error = %v, want wrapping %v (entry must be evicted)", err, authcode.ErrNotFound)
		}
	})

	t.Run("TTL boundary: a code that has not yet expired is still found and consumable", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		// Expires 2s in the future: comfortably larger than any
		// Save->FindByCode/Consume round-trip latency (including a real
		// Postgres round trip), while still exercising the "not yet
		// past expiresAt" edge rather than the full 10-minute TTL.
		ac := newAuthCodeExpiringAt(t, testCodeVerifierValue, 2*time.Second)
		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if _, err := repo.FindByCode(ctx, ac.Code()); err != nil {
			t.Fatalf("FindByCode() unexpected error: %v (code is still within its TTL)", err)
		}
		if err := repo.Consume(ctx, ac.Code()); err != nil {
			t.Fatalf("Consume() unexpected error: %v (code is still within its TTL)", err)
		}
	})

	t.Run("TTL boundary: a code that has already expired is neither found nor consumable", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ac := newAuthCodeExpiringAt(t, testCodeVerifierValue, -2*time.Second)
		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		if _, err := repo.FindByCode(ctx, ac.Code()); !errors.Is(err, authcode.ErrNotFound) {
			t.Errorf("FindByCode() error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
	})

	t.Run("Consume is atomic under concurrency: exactly one racer succeeds, every other observes ErrNotFound", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ac := newTestAuthCode(t, testCodeVerifierValue)
		if err := repo.Save(ctx, ac); err != nil {
			t.Fatalf("setup Save() unexpected error: %v", err)
		}

		const n = 20
		var successCount, notFoundCount, otherErrCount int64
		var wg sync.WaitGroup
		wg.Add(n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				<-start // every goroutine begins racing at (as close as possible to) the same instant
				err := repo.Consume(ctx, ac.Code())
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
	})
}

// newTestAuthCode builds a fresh AuthorizationCode (expiresAt =
// time.Now() + authcode.TTL, via authcode.New) whose PKCE
// code_challenge was derived from codeVerifier, so
// Challenge().Verify(codeVerifier) succeeds.
func newTestAuthCode(t *testing.T, codeVerifier string) *authcode.AuthorizationCode {
	t.Helper()

	scope, err := authcode.ParseScope("openid profile")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}
	challenge, err := authcode.NewCodeChallenge(deriveS256Challenge(codeVerifier), authcode.CodeChallengeMethodS256)
	if err != nil {
		t.Fatalf("setup NewCodeChallenge() unexpected error: %v", err)
	}

	ac, err := authcode.New(
		authcode.NewClientID("client-1"),
		authcode.NewUserID("user-1"),
		authcode.NewRedirectURI("http://localhost/callback"),
		scope,
		authcode.NewNonce("nonce-value"),
		challenge,
	)
	if err != nil {
		t.Fatalf("setup New() unexpected error: %v", err)
	}
	return ac
}

// newAuthCodeExpiringAt returns an AuthorizationCode identical to one
// built by newTestAuthCode, except its expiresAt is fixed to
// time.Now().Add(offset) via authcode.Reconstruct -- never via
// sleeping. A negative offset produces an already-expired fixture; a
// positive one produces one that expires imminently but has not yet.
func newAuthCodeExpiringAt(t *testing.T, codeVerifier string, offset time.Duration) *authcode.AuthorizationCode {
	t.Helper()
	base := newTestAuthCode(t, codeVerifier)
	return authcode.Reconstruct(
		base.Code(), base.ClientID(), base.UserID(), base.RedirectURI(),
		base.Scope(), base.Nonce(), base.Challenge(),
		time.Now().Add(offset), false,
	)
}

func deriveS256Challenge(codeVerifier string) string {
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// assertSameAuthCode compares every observable field of got against
// want.
//
// The raw PKCE challenge string has no exported accessor on
// CodeChallenge (only Method() does): a round trip through storage is
// instead verified indirectly, through the domain's own
// CodeChallenge.Verify method, using the codeVerifier that produced
// it at issuance. A mismatched or truncated persisted challenge would
// make this fail with ErrPKCEVerificationFailed -- arguably a
// stronger check than raw string equality, since it exercises the
// actual PKCE redemption path (service.AuthorizationService.Token ->
// AuthorizationCode.Verify) rather than an internal field.
//
// ExpiresAt is compared with microsecond truncation: Postgres's
// timestamptz column has microsecond precision, while Go's
// time.Now() carries nanoseconds, so an exact time.Time == comparison
// would spuriously fail once this contract is exercised against a
// real database.
func assertSameAuthCode(t *testing.T, got, want *authcode.AuthorizationCode, codeVerifier string) {
	t.Helper()
	if got.Code() != want.Code() {
		t.Errorf("Code() = %v, want %v", got.Code(), want.Code())
	}
	if got.ClientID() != want.ClientID() {
		t.Errorf("ClientID() = %v, want %v", got.ClientID(), want.ClientID())
	}
	if got.UserID() != want.UserID() {
		t.Errorf("UserID() = %v, want %v", got.UserID(), want.UserID())
	}
	if got.RedirectURI() != want.RedirectURI() {
		t.Errorf("RedirectURI() = %v, want %v", got.RedirectURI(), want.RedirectURI())
	}
	if got.Scope().String() != want.Scope().String() {
		t.Errorf("Scope() = %v, want %v", got.Scope(), want.Scope())
	}
	if got.Nonce() != want.Nonce() {
		t.Errorf("Nonce() = %v, want %v", got.Nonce(), want.Nonce())
	}
	if got.Challenge().Method() != want.Challenge().Method() {
		t.Errorf("Challenge().Method() = %v, want %v", got.Challenge().Method(), want.Challenge().Method())
	}
	if err := got.Challenge().Verify(codeVerifier); err != nil {
		t.Errorf("Challenge().Verify(codeVerifier) = %v, want nil (the persisted challenge must satisfy the same code_verifier it was issued with)", err)
	}
	if got.Consumed() != want.Consumed() {
		t.Errorf("Consumed() = %v, want %v", got.Consumed(), want.Consumed())
	}
	if !got.ExpiresAt().Truncate(time.Microsecond).Equal(want.ExpiresAt().Truncate(time.Microsecond)) {
		t.Errorf("ExpiresAt() = %v, want %v", got.ExpiresAt(), want.ExpiresAt())
	}
}
