package refreshtoken

import "context"

// Reader is the query half of the RefreshToken aggregate's
// persistence boundary (SPEC-010 R1: CQRS read/write port split).
//
// FindByTokenHash is a correctness-critical read: it is the reuse
// -detection pre-check performed just before Rotate, so the
// composition root MUST wire it to the writer connection pool rather
// than a read replica -- see docs/plans/SPEC-010-plan.md's "auth の
// correctness-critical read の配置" table. Reader itself carries no
// pool assignment; that is a wiring concern, not a domain one.
type Reader interface {
	// FindByTokenHash looks up a refresh token by the SHA-256 hash of
	// its opaque value. It returns:
	//   - the RefreshToken (which MAY be Consumed()==true) when a row
	//     exists AND is not expired -- consumed-but-unexpired rows are
	//     returned on purpose, so the service can detect a reuse of an
	//     already-rotated token before it even reaches Rotate;
	//   - a wrapped ErrNotFound when no row exists, OR the row has
	//     expired. Expired rows are lazily evicted as a side effect
	//     (same lazy-eviction contract as authcode.FindByCode), so
	//     expired == absent from the caller's point of view.
	FindByTokenHash(ctx context.Context, hash TokenHash) (*RefreshToken, error)
}

// Writer is the command half of the RefreshToken aggregate's
// persistence boundary (SPEC-010 R1).
//
// Rotate is the sole atomic single-use + rotation mechanism (the
// RefreshToken analogue of authcode.Writer.Consume): it flips the
// old token to consumed and inserts the new one in one critical
// section, so that when two callers race to rotate the same refresh
// token, exactly one wins (nil) and every loser observes ErrReused --
// the signal service.AuthorizationService uses to revoke the whole
// family (RFC 9700 4.14 reuse detection).
type Writer interface {
	// Save inserts rt as a new row (the initial refresh token minted at
	// authorization_code exchange, RefreshToken.Issue). It is a plain
	// insert, not an upsert: a token_hash collision would indicate a
	// broken random generator, not a legitimate re-save.
	Save(ctx context.Context, rt *RefreshToken) error

	// Rotate atomically consumes the token identified by oldHash and
	// inserts newRT, in a single transaction/critical section. It
	// returns:
	//   - nil when the old token existed, was NOT consumed and NOT
	//     expired: it is marked consumed and newRT is inserted (the
	//     caller may return the new refresh token to the client);
	//   - a wrapped ErrReused when a non-expired row for oldHash exists
	//     but could not be consumed because it was ALREADY consumed --
	//     this is both a genuine reuse (a replay after rotation) and the
	//     losing side of a concurrent Rotate race for the same token.
	//     The caller MUST revoke the whole family;
	//   - a wrapped ErrNotFound when no row for oldHash exists, or it
	//     has expired (there is no live family to protect: the caller
	//     returns invalid_grant WITHOUT revoking a family).
	//
	// Precedence when the atomic consume affects zero rows: if a
	// non-expired row for oldHash still exists it is necessarily
	// consumed -> ErrReused (reuse detection takes precedence);
	// otherwise (absent or expired) -> ErrNotFound. newRT is inserted
	// only on the nil path; ErrReused/ErrNotFound leave the store
	// unchanged (no partial write).
	Rotate(ctx context.Context, oldHash TokenHash, newRT *RefreshToken) error

	// RevokeFamily deletes every refresh token whose familyID matches
	// (the whole rotation chain). It is the reuse-detection response
	// (RFC 9700 4.14): after a reuse is detected, the entire family is
	// invalidated so a stolen token cannot yield further tokens. It is
	// idempotent -- deleting zero rows is not an error.
	RevokeFamily(ctx context.Context, familyID FamilyID) error
}

// Repository is the persistence boundary for the RefreshToken
// aggregate: the composition of Reader and Writer. It is declared in
// the domain layer (dependency inversion): infra/memory and
// infra/postgres provide interchangeable implementations whose
// observable behavior MUST be identical (proven by
// repotest.RunRefreshTokenRepositoryContract). Kept as the additive
// union of Reader/Writer (rather than replaced by them) because every
// current consumer -- service.AuthorizationService, repotest,
// infra/memory, infra/postgres -- still depends on it as a single
// interface; the composition root is what decides whether a
// Reader/Writer pair maps to one shared implementation or two
// pool-scoped ones (SPEC-010).
type Repository interface {
	Reader
	Writer
}
