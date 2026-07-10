package refreshtoken

import "errors"

// Sentinel errors returned by the refreshtoken domain package. Callers
// should use errors.Is to branch on these, since they may be wrapped
// with additional context via fmt.Errorf("...: %w", err).
var (
	// ErrNotFound is returned by Repository lookups (FindByTokenHash,
	// Rotate) when no matching, non-expired RefreshToken exists. It is
	// also returned by Rotate for a row that exists but has expired:
	// there is no live rotation family to protect, so the caller
	// reports invalid_grant without revoking a family (see
	// Repository.Rotate's doc comment for the full precedence rule).
	ErrNotFound = errors.New("refreshtoken: not found")

	// ErrExpired is a reserved sentinel for a RefreshToken past its
	// expiresAt. It parallels authcode.ErrExpired for symmetry, but
	// (unlike authcode) this domain's Repository contract never
	// surfaces it directly: an expired row is treated as absent
	// (ErrNotFound) by both FindByTokenHash and Rotate, per SPEC-006
	// plan §付録A's lazy-eviction contract.
	ErrExpired = errors.New("refreshtoken: expired")

	// ErrReused is returned by Repository.Rotate when the token
	// identified by oldHash exists, is not expired, but was already
	// consumed by a prior Rotate -- either a genuine replay of an
	// already-rotated refresh token, or the losing side of a
	// concurrent Rotate race. The caller MUST revoke the whole
	// rotation family (RFC 9700 4.14).
	ErrReused = errors.New("refreshtoken: reused")

	// ErrClientMismatch is returned when the client_id presented at
	// the token endpoint does not match the client this RefreshToken
	// was issued to (RFC 6749 6, SPEC-006 R6).
	ErrClientMismatch = errors.New("refreshtoken: client mismatch")

	// ErrInvalidScope is returned when a scope string cannot be parsed
	// (e.g. it is empty), or when Scope.Narrow is asked to widen the
	// original grant (SPEC-006 R7).
	ErrInvalidScope = errors.New("refreshtoken: invalid scope")

	// ErrInvalidToken is returned when a Token/TokenHash/FamilyID
	// cannot be parsed from an empty string.
	ErrInvalidToken = errors.New("refreshtoken: invalid token")
)
