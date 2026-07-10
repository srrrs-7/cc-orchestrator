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

	// ErrUnsupportedResponseType is returned when a Client does not
	// support the requested OAuth response_type.
	ErrUnsupportedResponseType = errors.New("client: unsupported response type")

	// ErrUnsupportedGrantType is returned when a Client does not
	// support the requested OAuth grant_type.
	ErrUnsupportedGrantType = errors.New("client: unsupported grant type")
)
