package client

import (
	"fmt"
	"net/url"
)

// RedirectURI is a value object representing a Client's registered
// redirection endpoint. It guarantees, at construction time, that the
// value is a well-formed absolute URI with an http/https scheme, as
// required for OAuth 2.0 redirect_uri comparison (RFC 6749 3.1.2).
type RedirectURI struct {
	value string
}

// NewRedirectURI validates s and constructs a RedirectURI. It returns
// ErrInvalidRedirectURI if s is not a well-formed absolute URI with an
// http or https scheme.
func NewRedirectURI(s string) (RedirectURI, error) {
	u, err := url.Parse(s)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return RedirectURI{}, fmt.Errorf("client: new redirect uri: %w", ErrInvalidRedirectURI)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return RedirectURI{}, fmt.Errorf("client: new redirect uri: %w", ErrInvalidRedirectURI)
	}
	return RedirectURI{value: s}, nil
}

// String returns the underlying string representation of the
// RedirectURI.
func (r RedirectURI) String() string {
	return r.value
}

// Equal reports whether r and other represent the same redirect URI.
// OAuth 2.0 requires exact string comparison of redirect_uri values
// (RFC 6749 3.1.2.3), not URI-normalized comparison.
func (r RedirectURI) Equal(other RedirectURI) bool {
	return r.value == other.value
}
