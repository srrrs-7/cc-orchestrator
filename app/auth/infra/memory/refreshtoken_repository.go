package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// RefreshTokenRepository is an in-memory, concurrency-safe
// implementation of refreshtoken.Repository.
//
// mu is a plain sync.Mutex rather than sync.RWMutex, mirroring
// AuthCodeRepository: FindByTokenHash must be able to atomically
// delete an expired entry (lazy eviction) and Rotate must atomically
// check-then-mutate two map entries (old -> consumed, new -> inserted)
// in one critical section, so every access path needs exclusive
// access.
type RefreshTokenRepository struct {
	mu     sync.Mutex
	tokens map[refreshtoken.TokenHash]*refreshtoken.RefreshToken
}

// The following var _ declarations verify at compile time that
// RefreshTokenRepository satisfies the domain's Repository interface,
// and -- SPEC-010 R5/R1 -- both of its additive halves (Reader/Writer)
// individually, via this single struct/store (in-memory persistence
// is not physically split into separate reader/writer pools).
var (
	_ refreshtoken.Repository = (*RefreshTokenRepository)(nil)
	_ refreshtoken.Reader     = (*RefreshTokenRepository)(nil)
	_ refreshtoken.Writer     = (*RefreshTokenRepository)(nil)
)

// NewRefreshTokenRepository builds an empty RefreshTokenRepository.
func NewRefreshTokenRepository() *RefreshTokenRepository {
	return &RefreshTokenRepository{
		tokens: make(map[refreshtoken.TokenHash]*refreshtoken.RefreshToken),
	}
}

// Save inserts rt into the store. A clone is stored so that later
// mutations to the caller's *refreshtoken.RefreshToken do not leak
// into the repository's internal state (mirrors
// AuthCodeRepository.Save).
func (r *RefreshTokenRepository) Save(ctx context.Context, rt *refreshtoken.RefreshToken) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("memory: save refresh token: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tokens[rt.TokenHash()] = cloneRefreshToken(rt)
	return nil
}

// FindByTokenHash returns the RefreshToken stored under hash,
// including a consumed-but-unexpired one (reuse detection needs to
// observe it -- see refreshtoken.Repository.FindByTokenHash's doc
// comment). It returns refreshtoken.ErrNotFound when hash is absent,
// or when the stored entry has already expired -- an expired entry is
// evicted as a side effect (lazy cleanup), same as
// AuthCodeRepository.FindByCode.
func (r *RefreshTokenRepository) FindByTokenHash(ctx context.Context, hash refreshtoken.TokenHash) (*refreshtoken.RefreshToken, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find refresh token: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	rt, ok := r.tokens[hash]
	if !ok {
		return nil, fmt.Errorf("memory: find refresh token: %w", refreshtoken.ErrNotFound)
	}
	if rt.IsExpired() {
		delete(r.tokens, hash)
		return nil, fmt.Errorf("memory: find refresh token: %w", refreshtoken.ErrNotFound)
	}
	return cloneRefreshToken(rt), nil
}

// Rotate implements refreshtoken.Repository's atomic single-use +
// rotation contract within one r.mu critical section (the RefreshToken
// analogue of AuthCodeRepository.Consume): it looks oldHash up, checks
// expiry and consumed state, and -- only if the entry exists, is not
// expired, and is not already consumed -- flips it to consumed and
// inserts newRT under its own hash, all before releasing the lock.
// Whichever caller's Rotate executes that flip is the only winner;
// every other concurrent or subsequent caller for the same oldHash
// observes ErrReused here (SPEC-006 付録 A's precedence: a still-live,
// already-consumed row always reports ErrReused, never ErrNotFound).
func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldHash refreshtoken.TokenHash, newRT *refreshtoken.RefreshToken) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("memory: rotate refresh token: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	old, ok := r.tokens[oldHash]
	if !ok {
		return fmt.Errorf("memory: rotate refresh token: %w", refreshtoken.ErrNotFound)
	}
	if old.IsExpired() {
		delete(r.tokens, oldHash)
		return fmt.Errorf("memory: rotate refresh token: %w", refreshtoken.ErrNotFound)
	}
	if old.Consumed() {
		return fmt.Errorf("memory: rotate refresh token: %w", refreshtoken.ErrReused)
	}

	// The RefreshToken aggregate has no Consume-like mutator (only
	// Repository.Rotate is the authoritative state transition -- see
	// domain/refreshtoken/refresh_token.go's Rotate doc comment), so
	// the old entry's consumed state is applied by rebuilding it via
	// Reconstruct(..., consumed=true) and replacing the map entry,
	// rather than mutating old in place.
	r.tokens[oldHash] = consumedRefreshToken(old)
	r.tokens[newRT.TokenHash()] = cloneRefreshToken(newRT)
	return nil
}

// RevokeFamily deletes every token whose FamilyID matches familyID.
// It is idempotent: a family with no rows is not an error.
func (r *RefreshTokenRepository) RevokeFamily(ctx context.Context, familyID refreshtoken.FamilyID) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("memory: revoke refresh token family: %w", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for hash, rt := range r.tokens {
		if rt.FamilyID() == familyID {
			delete(r.tokens, hash)
		}
	}
	return nil
}

// cloneRefreshToken returns a new *refreshtoken.RefreshToken built
// from rt's state via refreshtoken.Reconstruct, preventing the
// repository's stored data and the caller's data from aliasing
// (sharing) the same mutable object (mirrors cloneAuthCode).
func cloneRefreshToken(rt *refreshtoken.RefreshToken) *refreshtoken.RefreshToken {
	return refreshtoken.Reconstruct(
		rt.TokenHash(),
		rt.FamilyID(),
		rt.ClientID(),
		rt.UserID(),
		rt.Scope(),
		rt.ExpiresAt(),
		rt.Consumed(),
	)
}

// consumedRefreshToken returns a clone of rt with consumed forced to
// true, used by Rotate to flip the old entry's state (see Rotate's
// doc comment for why this goes through Reconstruct rather than a
// mutator).
func consumedRefreshToken(rt *refreshtoken.RefreshToken) *refreshtoken.RefreshToken {
	return refreshtoken.Reconstruct(
		rt.TokenHash(),
		rt.FamilyID(),
		rt.ClientID(),
		rt.UserID(),
		rt.Scope(),
		rt.ExpiresAt(),
		true,
	)
}
