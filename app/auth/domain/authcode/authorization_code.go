package authcode

import "time"

// TTL is the lifetime of a newly issued AuthorizationCode.
// Authorization codes MUST be short-lived (RFC 6749 4.1.2 recommends
// a maximum lifetime of 10 minutes); this authorization server uses
// exactly that value.
const TTL = 10 * time.Minute

// ClientID, UserID and RedirectURI below are thin, string-backed
// value objects that let the AuthorizationCode aggregate reference
// the client and user bounded contexts *by value* without importing
// the client/user packages (see the "no cross-aggregate package
// dependency" rule for this module). They are populated by the
// application layer (service) from the corresponding
// client.ClientID / user.UserID / client.RedirectURI values.

// ClientID identifies, by value, the Client an AuthorizationCode was
// issued to.
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

// UserID identifies, by value, the User (resource owner) an
// AuthorizationCode was issued for. Its value is also used as the
// OIDC "sub" claim by the application layer.
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

// RedirectURI identifies, by value, the redirect_uri an
// AuthorizationCode is bound to (RFC 6749 4.1.3: the token endpoint
// MUST verify that the redirect_uri parameter matches the one used in
// the authorization request).
type RedirectURI struct {
	value string
}

// NewRedirectURI wraps s as a RedirectURI.
func NewRedirectURI(s string) RedirectURI {
	return RedirectURI{value: s}
}

// String returns the underlying string representation of the
// RedirectURI.
func (r RedirectURI) String() string {
	return r.value
}

// AuthorizationCode is the aggregate root representing a single,
// opaque, single-use authorization code issued from the /authorize
// endpoint and redeemable exactly once at the /token endpoint (RFC
// 6749 4.1).
type AuthorizationCode struct {
	code        Code
	clientID    ClientID
	userID      UserID
	redirectURI RedirectURI
	scope       Scope
	nonce       Nonce
	challenge   CodeChallenge
	expiresAt   time.Time
	consumed    bool
}

// New is the factory for issuing a brand new AuthorizationCode. It
// generates a fresh opaque Code and fixes expiresAt to time.Now() +
// TTL at creation time; the code starts unconsumed.
func New(clientID ClientID, userID UserID, redirectURI RedirectURI, scope Scope, nonce Nonce, challenge CodeChallenge) (*AuthorizationCode, error) {
	code, err := NewCode()
	if err != nil {
		return nil, err
	}
	return &AuthorizationCode{
		code:        code,
		clientID:    clientID,
		userID:      userID,
		redirectURI: redirectURI,
		scope:       scope,
		nonce:       nonce,
		challenge:   challenge,
		expiresAt:   time.Now().Add(TTL),
		consumed:    false,
	}, nil
}

// Reconstruct rebuilds an AuthorizationCode from already-validated
// persisted state. It is intended to be used exclusively by
// infrastructure-layer repository implementations when loading an
// AuthorizationCode from storage.
func Reconstruct(code Code, clientID ClientID, userID UserID, redirectURI RedirectURI, scope Scope, nonce Nonce, challenge CodeChallenge, expiresAt time.Time, consumed bool) *AuthorizationCode {
	return &AuthorizationCode{
		code:        code,
		clientID:    clientID,
		userID:      userID,
		redirectURI: redirectURI,
		scope:       scope,
		nonce:       nonce,
		challenge:   challenge,
		expiresAt:   expiresAt,
		consumed:    consumed,
	}
}

// Verify checks every condition the token endpoint MUST enforce
// before an AuthorizationCode may be redeemed (RFC 6749 4.1.3, RFC
// 7636 4.6): it must not already be consumed, it must not be
// expired, the presented redirectURI/clientID must match the values
// it was bound to at issuance, and codeVerifier must satisfy the
// bound PKCE code_challenge. It does not mutate the aggregate or mark
// it as consumed; call Consume separately once the caller is ready to
// commit to redeeming it.
func (a *AuthorizationCode) Verify(codeVerifier string, redirectURI RedirectURI, clientID ClientID) error {
	if a.consumed {
		return ErrAlreadyConsumed
	}
	if a.IsExpired() {
		return ErrExpired
	}
	if a.redirectURI != redirectURI {
		return ErrRedirectURIMismatch
	}
	if a.clientID != clientID {
		return ErrClientMismatch
	}
	if err := a.challenge.Verify(codeVerifier); err != nil {
		return err
	}
	return nil
}

// On this module's production request path, single-use enforcement is
// owned entirely by Repository.Consume (delete-based); this method,
// the consumed field, and ErrAlreadyConsumed are never exercised
// there.
//
// Consume marks the AuthorizationCode as used. It returns
// ErrAlreadyConsumed if it was already consumed.
//
// This in-memory flag is NOT what enforces single-use semantics (RFC
// 6749 4.1.2: "The client MUST NOT use the authorization code more
// than once") for concurrent redemption attempts: a *AuthorizationCode
// value handed to a caller (e.g. by Repository.FindByCode) is a
// disconnected clone, so two callers racing on the same underlying
// code would each call Consume on their own copy and each succeed.
// The repository's Repository.Consume is the atomic, authoritative
// guarantee (see service.AuthorizationService.Token, which calls
// Verify on the aggregate for its read-only correctness checks but
// delegates actual redemption to Repository.Consume). This method is
// kept for completeness of the aggregate's behavioral API and as a
// defense-in-depth check for any single-threaded/direct use.
func (a *AuthorizationCode) Consume() error {
	if a.consumed {
		return ErrAlreadyConsumed
	}
	a.consumed = true
	return nil
}

// IsExpired reports whether the AuthorizationCode's TTL has elapsed.
func (a *AuthorizationCode) IsExpired() bool {
	return time.Now().After(a.expiresAt)
}

// Code returns the AuthorizationCode's opaque identifier.
func (a *AuthorizationCode) Code() Code {
	return a.code
}

// ClientID returns the Client this code was issued to.
func (a *AuthorizationCode) ClientID() ClientID {
	return a.clientID
}

// UserID returns the resource owner this code was issued for.
func (a *AuthorizationCode) UserID() UserID {
	return a.userID
}

// RedirectURI returns the redirect_uri this code is bound to.
func (a *AuthorizationCode) RedirectURI() RedirectURI {
	return a.redirectURI
}

// Scope returns the requested (and granted) scope.
func (a *AuthorizationCode) Scope() Scope {
	return a.scope
}

// Nonce returns the OIDC nonce bound to this code, if any.
func (a *AuthorizationCode) Nonce() Nonce {
	return a.nonce
}

// Challenge returns the PKCE code_challenge bound to this code.
// Exposed primarily so infrastructure-layer repositories can
// reconstruct a clone of the aggregate for storage isolation.
func (a *AuthorizationCode) Challenge() CodeChallenge {
	return a.challenge
}

// Consumed reports whether the AuthorizationCode has already been
// redeemed.
func (a *AuthorizationCode) Consumed() bool {
	return a.consumed
}

// ExpiresAt returns the time at which the AuthorizationCode expires.
func (a *AuthorizationCode) ExpiresAt() time.Time {
	return a.expiresAt
}
