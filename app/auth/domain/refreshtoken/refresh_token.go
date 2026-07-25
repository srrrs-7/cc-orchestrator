package refreshtoken

import "time"

// RefreshTokenTTL is the lifetime of a newly issued or freshly
// rotated RefreshToken. It is sliding: every successful Rotate resets
// expiresAt to time.Now() + RefreshTokenTTL (SPEC-006 非機能要件 "TTL は
// 30 日。ローテーションごとにリセット").
const RefreshTokenTTL = 30 * 24 * time.Hour

// ClientID and UserID below are thin, string-backed value objects that
// let the RefreshToken aggregate reference the client/user bounded
// contexts *by value*, without importing the client/user packages
// (mirrors domain/authcode's ClientID/UserID -- see its doc comment
// for the "no cross-aggregate package dependency" rule this module
// follows). They are populated by the application layer (service)
// from the corresponding client.ClientID / user.UserID values.

// ClientID identifies, by value, the Client a RefreshToken was issued
// to.
type ClientID struct {
	value string
}

// NewClientID wraps s as a ClientID.
func NewClientID(s string) ClientID {
	return ClientID{value: s}
}

// String returns the underlying string representation of the
// ClientID.
func (c ClientID) String() string {
	return c.value
}

// UserID identifies, by value, the User (resource owner) a
// RefreshToken was issued for.
type UserID struct {
	value string
}

// NewUserID wraps s as a UserID.
func NewUserID(s string) UserID {
	return UserID{value: s}
}

// String returns the underlying string representation of the UserID.
func (u UserID) String() string {
	return u.value
}

// RefreshToken is the aggregate root representing a single, opaque
// refresh token issued to a client on behalf of a user (RFC 6749 6).
// It only ever exposes/derives a TokenHash: the plaintext Token
// returned by Issue/Rotate is a separate, one-time-use value the
// aggregate itself never stores (SPEC-006 R8).
type RefreshToken struct {
	tokenHash TokenHash
	familyID  FamilyID
	clientID  ClientID
	userID    UserID
	scope     Scope
	authTime  time.Time // IdP session login time (OIDC auth_time); zero when unavailable
	expiresAt time.Time
	consumed  bool
}

// Issue is the factory for a brand new RefreshToken, minted at
// authorization_code exchange time (SPEC-006 R2): a fresh Token/
// TokenHash, a fresh FamilyID (the start of a new rotation chain),
// expiresAt = now + RefreshTokenTTL, and consumed=false. It returns
// the aggregate together with the one-time plaintext Token the caller
// must return to the client. authTime is the IdP session login
// timestamp carried forward into the refresh token so it can be
// re-used when re-issuing ID tokens on subsequent refresh grants
// (OIDC Core auth_time); a zero time.Time is valid.
func Issue(clientID ClientID, userID UserID, scope Scope, authTime time.Time) (*RefreshToken, Token, error) {
	plaintext, err := NewToken()
	if err != nil {
		return nil, Token{}, err
	}
	familyID, err := NewFamilyID()
	if err != nil {
		return nil, Token{}, err
	}
	rt := &RefreshToken{
		tokenHash: plaintext.Hash(),
		familyID:  familyID,
		clientID:  clientID,
		userID:    userID,
		scope:     scope,
		authTime:  authTime,
		expiresAt: time.Now().Add(RefreshTokenTTL),
		consumed:  false,
	}
	return rt, plaintext, nil
}

// Rotate produces the next RefreshToken in rt's rotation chain (SPEC-006
// R4/R7): a fresh Token/TokenHash, the SAME FamilyID as rt (rotation
// never starts a new family), a fresh sliding expiresAt = now +
// RefreshTokenTTL, consumed=false, and scope set to the caller-supplied
// effective scope (the result of Scope.Narrow against rt's own scope,
// computed by the caller). authTime is carried forward unchanged from
// rt so every ID token in the chain reflects the original login time.
// It does not mutate rt; the actual atomic consume-old + insert-new
// state transition is owned by Repository.Rotate.
func (rt *RefreshToken) Rotate(scope Scope) (*RefreshToken, Token, error) {
	plaintext, err := NewToken()
	if err != nil {
		return nil, Token{}, err
	}
	newRT := &RefreshToken{
		tokenHash: plaintext.Hash(),
		familyID:  rt.familyID,
		clientID:  rt.clientID,
		userID:    rt.userID,
		scope:     scope,
		authTime:  rt.authTime,
		expiresAt: time.Now().Add(RefreshTokenTTL),
		consumed:  false,
	}
	return newRT, plaintext, nil
}

// Reconstruct rebuilds a RefreshToken from already-validated persisted
// state. It is intended to be used exclusively by infrastructure-layer
// repository implementations when loading a RefreshToken from
// storage. authTime is the IdP session login timestamp; pass
// time.Time{} when the column is not yet persisted in the DB.
func Reconstruct(hash TokenHash, familyID FamilyID, clientID ClientID, userID UserID, scope Scope, authTime time.Time, expiresAt time.Time, consumed bool) *RefreshToken {
	return &RefreshToken{
		tokenHash: hash,
		familyID:  familyID,
		clientID:  clientID,
		userID:    userID,
		scope:     scope,
		authTime:  authTime,
		expiresAt: expiresAt,
		consumed:  consumed,
	}
}

// MatchesClient reports whether clientID matches the Client this
// RefreshToken was issued to (RFC 6749 6, SPEC-006 R6). It returns
// ErrClientMismatch otherwise.
func (rt *RefreshToken) MatchesClient(clientID ClientID) error {
	if rt.clientID != clientID {
		return ErrClientMismatch
	}
	return nil
}

// IsExpired reports whether the RefreshToken's TTL has elapsed.
func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.expiresAt)
}

// TokenHash returns the SHA-256 hash this RefreshToken is stored/
// looked up by.
func (rt *RefreshToken) TokenHash() TokenHash {
	return rt.tokenHash
}

// FamilyID returns the rotation chain this RefreshToken belongs to.
func (rt *RefreshToken) FamilyID() FamilyID {
	return rt.familyID
}

// ClientID returns the Client this RefreshToken was issued to.
func (rt *RefreshToken) ClientID() ClientID {
	return rt.clientID
}

// UserID returns the resource owner this RefreshToken was issued for.
func (rt *RefreshToken) UserID() UserID {
	return rt.userID
}

// Scope returns this RefreshToken's granted (or, after a narrowing
// rotation, effective) scope.
func (rt *RefreshToken) Scope() Scope {
	return rt.scope
}

// AuthTime returns the IdP session login time carried in this refresh
// token (OIDC auth_time). A zero time.Time means the timestamp was
// not available at Issue time (e.g. loaded from a DB row that
// predates the auth_time column).
func (rt *RefreshToken) AuthTime() time.Time {
	return rt.authTime
}

// ExpiresAt returns the time at which the RefreshToken expires.
func (rt *RefreshToken) ExpiresAt() time.Time {
	return rt.expiresAt
}

// Consumed reports whether the RefreshToken has already been rotated
// (redeemed). A consumed-but-unexpired RefreshToken presented again is
// the reuse-detection signal (SPEC-006 R5).
func (rt *RefreshToken) Consumed() bool {
	return rt.consumed
}
