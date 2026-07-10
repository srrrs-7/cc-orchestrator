package authcode

import "errors"

// Sentinel errors returned by the authcode domain package. Callers
// should use errors.Is to branch on these, since they may be wrapped
// with additional context via fmt.Errorf("...: %w", err).
var (
	// ErrNotFound is returned by Repository lookups when no matching
	// AuthorizationCode exists.
	ErrNotFound = errors.New("authcode: not found")

	// ErrAlreadyConsumed is returned when Consume (or Verify) is
	// called on an AuthorizationCode that has already been consumed.
	// Authorization codes MUST be single-use (RFC 6749 4.1.2).
	ErrAlreadyConsumed = errors.New("authcode: already consumed")

	// ErrExpired is returned when Verify is called on an
	// AuthorizationCode past its expiresAt time.
	ErrExpired = errors.New("authcode: expired")

	// ErrRedirectURIMismatch is returned when the redirect_uri
	// presented at the token endpoint does not match the one bound to
	// the AuthorizationCode at issuance time.
	ErrRedirectURIMismatch = errors.New("authcode: redirect uri mismatch")

	// ErrClientMismatch is returned when the client_id presented at
	// the token endpoint does not match the one bound to the
	// AuthorizationCode at issuance time.
	ErrClientMismatch = errors.New("authcode: client mismatch")

	// ErrPKCEVerificationFailed is returned when the code_verifier
	// presented at the token endpoint does not satisfy the bound PKCE
	// code_challenge (RFC 7636 4.6).
	ErrPKCEVerificationFailed = errors.New("authcode: pkce verification failed")

	// ErrInvalidCodeVerifier is returned when a code_verifier does not
	// meet the length (43-128) or character-set (unreserved) rules of
	// RFC 7636 4.1.
	ErrInvalidCodeVerifier = errors.New("authcode: invalid code verifier")

	// ErrUnsupportedChallengeMethod is returned when a
	// code_challenge_method other than "S256" is requested. This
	// authorization server only accepts S256 (RFC 7636 / OAuth 2.1
	// best practice); "plain" is rejected as invalid_request.
	ErrUnsupportedChallengeMethod = errors.New("authcode: unsupported code challenge method")

	// ErrMissingOpenIDScope is returned when a requested scope does
	// not include "openid", which is REQUIRED for OIDC requests
	// (OIDC Core 3.1.2.1).
	ErrMissingOpenIDScope = errors.New("authcode: missing openid scope")

	// ErrInvalidScope is returned when a scope string cannot be parsed
	// (e.g. it is empty).
	ErrInvalidScope = errors.New("authcode: invalid scope")
)
