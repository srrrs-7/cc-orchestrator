package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

// AuthCodeRepository is an in-memory, concurrency-safe implementation
// of authcode.Repository.
//
// mu is a plain sync.Mutex rather than sync.RWMutex: both FindByCode
// and Consume must be able to atomically delete an entry (FindByCode
// to lazily evict an expired one; Consume to enforce single-use), so
// every access path needs exclusive access -- there is no read-only
// path left to benefit from a separate read lock.
type AuthCodeRepository struct {
	mu    sync.Mutex
	codes map[authcode.Code]*authcode.AuthorizationCode
}

// var _ authcode.Repository = (*AuthCodeRepository)(nil) verifies at
// compile time that AuthCodeRepository satisfies the domain's
// Repository interface.
var _ authcode.Repository = (*AuthCodeRepository)(nil)

// NewAuthCodeRepository builds an empty AuthCodeRepository.
func NewAuthCodeRepository() *AuthCodeRepository {
	return &AuthCodeRepository{
		codes: make(map[authcode.Code]*authcode.AuthorizationCode),
	}
}

// Save inserts or updates ac in the store. It is used to persist a
// newly issued code (see service.AuthorizationService.Authorize);
// the token endpoint's single-use redemption goes through Consume
// instead, which mutates the store directly. A clone is stored so
// that later mutations to the caller's *authcode.AuthorizationCode
// do not leak into the repository's internal state.
func (r *AuthCodeRepository) Save(ctx context.Context, ac *authcode.AuthorizationCode) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("memory: save authorization code: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.codes[ac.Code()] = cloneAuthCode(ac)
	return nil
}

// FindByCode returns the AuthorizationCode with the given code, or
// authcode.ErrNotFound if none exists. If the stored entry has
// already expired, it is evicted from the store as a side effect
// (lazy cleanup, so the map does not grow without bound just from
// abandoned/expired codes) and authcode.ErrNotFound is returned for
// it, same as a genuinely absent code.
func (r *AuthCodeRepository) FindByCode(ctx context.Context, code authcode.Code) (*authcode.AuthorizationCode, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find authorization code: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ac, ok := r.codes[code]
	if !ok {
		return nil, fmt.Errorf("memory: find authorization code: %w", authcode.ErrNotFound)
	}
	if ac.IsExpired() {
		delete(r.codes, code)
		return nil, fmt.Errorf("memory: find authorization code: %w", authcode.ErrNotFound)
	}
	return cloneAuthCode(ac), nil
}

// Consume implements authcode.Repository's atomic single-use
// contract: within one r.mu critical section it looks up code,
// checks expiry, and -- only if the entry exists and is not expired
// -- deletes it and returns nil. Deleting on success (rather than
// flipping a "consumed" flag and keeping the entry around) is what
// makes this both the sole authority for single-use enforcement
// (whichever caller's Consume executes the delete is the only
// winner; every other concurrent or subsequent caller for the same
// code, including one that already passed AuthorizationCode.Verify
// against its own now-stale clone, observes ErrNotFound here) and a
// natural bound on the store's size (redeemed codes do not linger).
func (r *AuthCodeRepository) Consume(ctx context.Context, code authcode.Code) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("memory: consume authorization code: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ac, ok := r.codes[code]
	if !ok {
		return fmt.Errorf("memory: consume authorization code: %w", authcode.ErrNotFound)
	}
	if ac.IsExpired() {
		delete(r.codes, code)
		return fmt.Errorf("memory: consume authorization code: %w", authcode.ErrExpired)
	}

	delete(r.codes, code)
	return nil
}

// cloneAuthCode returns a new *authcode.AuthorizationCode built from
// ac's state via authcode.Reconstruct, preventing the repository's
// stored data and the caller's data from aliasing (sharing) the same
// mutable object.
func cloneAuthCode(ac *authcode.AuthorizationCode) *authcode.AuthorizationCode {
	return authcode.Reconstruct(
		ac.Code(),
		ac.ClientID(),
		ac.UserID(),
		ac.RedirectURI(),
		ac.Scope(),
		ac.Nonce(),
		ac.Challenge(),
		ac.ExpiresAt(),
		ac.Consumed(),
	)
}
