package authcode

import "context"

// Reader is the query half of the AuthorizationCode aggregate's
// persistence boundary (SPEC-010 R1: CQRS read/write port split).
// FindByCode returns ErrNotFound when no matching AuthorizationCode
// exists (this includes an entry that has expired: implementations
// are expected to lazily evict expired entries so the store does not
// grow without bound, and to report them as not-found rather than as
// a distinct state).
//
// FindByCode is a correctness-critical read (a code is looked up
// immediately after being issued, at the /token exchange), so the
// composition root MUST wire it to the writer connection pool rather
// than a read replica -- see docs/plans/SPEC-010-plan.md's "auth の
// correctness-critical read の配置" table. Reader itself carries no
// pool assignment; that is a wiring concern, not a domain one.
type Reader interface {
	FindByCode(ctx context.Context, code Code) (*AuthorizationCode, error)
}

// Writer is the command half of the AuthorizationCode aggregate's
// persistence boundary (SPEC-010 R1). Save is used to persist a
// newly issued code.
//
// Consume is the sole mechanism for single-use enforcement and MUST
// be implemented atomically (a single critical section covering
// "does code still exist, is it not expired, then remove it"), so
// that when two callers race to redeem the same code, exactly one
// Consume call succeeds. Implementations are expected to delete the
// entry as part of a successful Consume (rather than merely flagging
// it consumed) so that redeemed/expired codes do not accumulate. See
// service.AuthorizationService.Token, which calls
// AuthorizationCode.Verify (read-only correctness checks: PKCE,
// redirect_uri, client_id) before calling Consume (the atomic,
// authoritative single-use guarantee).
type Writer interface {
	Save(ctx context.Context, ac *AuthorizationCode) error

	// Consume atomically claims code for one-time use. It returns:
	//   - nil if code existed, was not expired, and was successfully
	//     removed from the store (the caller may now issue tokens for
	//     it; a repeat Consume/FindByCode call for the same code will
	//     observe it as gone);
	//   - a wrapped ErrNotFound if no entry for code exists -- this is
	//     also what every losing side of a concurrent race for the
	//     same code observes, and what a genuine reuse (a second
	//     /token call after the code was already consumed) observes;
	//   - a wrapped ErrExpired if an entry exists but its TTL has
	//     elapsed (the entry is deleted as part of returning this
	//     error, same lazy-eviction behavior as FindByCode).
	Consume(ctx context.Context, code Code) error
}

// Repository is the persistence boundary for the AuthorizationCode
// aggregate: the composition of Reader and Writer. It is defined in
// the domain layer (dependency inversion): the domain declares what
// it needs, and the infrastructure layer provides a concrete
// implementation. Kept as the additive union of Reader/Writer (rather
// than replaced by them) because every current consumer --
// service.AuthorizationService, repotest, infra/memory,
// infra/postgres -- still depends on it as a single interface; the
// composition root is what decides whether a Reader/Writer pair maps
// to one shared implementation or two pool-scoped ones (SPEC-010).
type Repository interface {
	Reader
	Writer
}
