package client

import "errors"

// Sentinel errors returned by the client domain package. Callers should
// use errors.Is to branch on these, since they may be wrapped with
// additional context via fmt.Errorf("...: %w", err).
var (
	// ErrNotFound is returned by Repository lookups when no matching
	// Client exists.
	ErrNotFound = errors.New("client: not found")

	// ErrInvalidClientID is returned when a ClientID cannot be parsed
	// from a string (e.g. it is empty).
	ErrInvalidClientID = errors.New("client: invalid client id")

	// ErrInvalidRedirectURI is returned when a RedirectURI is not a
	// well-formed absolute URI.
	ErrInvalidRedirectURI = errors.New("client: invalid redirect uri")

	// ErrRedirectURIMismatch is returned when a requested redirect_uri
	// does not match any of the Client's registered redirect URIs.
	ErrRedirectURIMismatch = errors.New("client: redirect uri mismatch")

	// ErrMissingResponseType is returned when the authorize request
	// omits the required response_type parameter (RFC 6749 §4.1.2.1).
	ErrMissingResponseType = errors.New("client: missing response type")

	// ErrUnsupportedResponseType is returned when a Client does not
	// support the requested OAuth response_type.
	ErrUnsupportedResponseType = errors.New("client: unsupported response type")

	// ErrUnsupportedGrantType is returned when a Client does not
	// support the requested OAuth grant_type.
	ErrUnsupportedGrantType = errors.New("client: unsupported grant type")

	// ErrClientAuthFailed is returned when a confidential client
	// presents an incorrect or missing client_secret at the token or
	// revocation endpoint (RFC 6749 2.3.1, 5.2 invalid_client).
	ErrClientAuthFailed = errors.New("client: authentication failed")
)
