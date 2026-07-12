// Package client contains the Client aggregate and its supporting
// domain types (value objects, repository interface and domain
// errors). This package has no dependency on any other layer, and it
// does not depend on any other bounded context (user / authcode /
// token): cross-aggregate references elsewhere in this system are
// expressed as plain IDs/values, never as a direct reference to a
// *Client.
package client

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// Client is the aggregate root representing a registered OAuth 2.0
// client application. It supports both public clients (RFC 6749 2.1,
// no client secret, token_endpoint_auth_method=none) and confidential
// clients (RFC 6749 2.1, authenticated with a bcrypt-hashed
// client_secret via client_secret_post or client_secret_basic).
// secretHash nil means public client.
type Client struct {
	id            ClientID
	secretHash    *string // nil = public client; non-nil = confidential client (bcrypt hash)
	redirectURIs  []RedirectURI
	allowedScopes map[string]struct{}
	responseTypes map[string]struct{}
	grantTypes    map[string]struct{}
}

// New is the factory for registering a brand new public Client
// (token_endpoint_auth_method=none). For confidential clients, use
// NewConfidential.
func New(id ClientID, redirectURIs []RedirectURI, allowedScopes, responseTypes, grantTypes []string) *Client {
	return &Client{
		id:            id,
		redirectURIs:  append([]RedirectURI(nil), redirectURIs...),
		allowedScopes: toSet(allowedScopes),
		responseTypes: toSet(responseTypes),
		grantTypes:    toSet(grantTypes),
	}
}

// NewConfidential is the factory for registering a brand new
// confidential Client (RFC 6749 2.1). plaintextSecret is hashed with
// bcrypt before storage; callers must never persist the returned hash
// via SecretHash() outside infrastructure seed paths.
func NewConfidential(id ClientID, redirectURIs []RedirectURI, allowedScopes, responseTypes, grantTypes []string, plaintextSecret string) (*Client, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextSecret), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("client: hash secret: %w", err)
	}
	h := string(hash)
	return &Client{
		id:            id,
		secretHash:    &h,
		redirectURIs:  append([]RedirectURI(nil), redirectURIs...),
		allowedScopes: toSet(allowedScopes),
		responseTypes: toSet(responseTypes),
		grantTypes:    toSet(grantTypes),
	}, nil
}

// Reconstruct rebuilds a Client from already-validated persisted
// state. secretHash must be the bcrypt hash loaded from storage (nil
// for public clients). It is intended to be used exclusively by
// infrastructure-layer repository implementations when loading a
// Client from storage.
func Reconstruct(id ClientID, redirectURIs []RedirectURI, allowedScopes, responseTypes, grantTypes []string, secretHash *string) *Client {
	c := New(id, redirectURIs, allowedScopes, responseTypes, grantTypes)
	c.secretHash = secretHash
	return c
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}

// ID returns the Client's identifier.
func (c *Client) ID() ClientID {
	return c.id
}

// RedirectURIs returns a copy of the Client's registered redirect
// URIs.
func (c *Client) RedirectURIs() []RedirectURI {
	return append([]RedirectURI(nil), c.redirectURIs...)
}

// ValidateRedirectURI reports whether uri exactly matches one of the
// Client's registered redirect URIs. It returns
// ErrRedirectURIMismatch otherwise.
func (c *Client) ValidateRedirectURI(uri RedirectURI) error {
	for _, registered := range c.redirectURIs {
		if registered.Equal(uri) {
			return nil
		}
	}
	return ErrRedirectURIMismatch
}

// SupportsResponseType reports whether the Client is registered to
// use the given OAuth response_type (e.g. "code").
func (c *Client) SupportsResponseType(responseType string) bool {
	_, ok := c.responseTypes[responseType]
	return ok
}

// SupportsGrantType reports whether the Client is registered to use
// the given OAuth grant_type (e.g. "authorization_code").
func (c *Client) SupportsGrantType(grantType string) bool {
	_, ok := c.grantTypes[grantType]
	return ok
}

// AllowsScope reports whether the Client is permitted to request the
// given scope value.
func (c *Client) AllowsScope(scope string) bool {
	_, ok := c.allowedScopes[scope]
	return ok
}

// AllowedScopes returns the Client's permitted scope values. Exposed
// primarily so infrastructure-layer repositories can reconstruct a
// clone of the aggregate for storage isolation.
func (c *Client) AllowedScopes() []string {
	return fromSet(c.allowedScopes)
}

// ResponseTypes returns the Client's supported OAuth response_type
// values. Exposed primarily so infrastructure-layer repositories can
// reconstruct a clone of the aggregate for storage isolation.
func (c *Client) ResponseTypes() []string {
	return fromSet(c.responseTypes)
}

// GrantTypes returns the Client's supported OAuth grant_type values.
// Exposed primarily so infrastructure-layer repositories can
// reconstruct a clone of the aggregate for storage isolation.
func (c *Client) GrantTypes() []string {
	return fromSet(c.grantTypes)
}

// IsConfidential reports whether this Client requires secret
// authentication at the token endpoint (RFC 6749 2.1).
func (c *Client) IsConfidential() bool {
	return c.secretHash != nil
}

// VerifySecret reports whether candidate matches the Client's stored
// bcrypt hash using constant-time comparison via bcrypt. Always returns
// true for public clients (no secret required). Returns false for
// confidential clients when candidate is empty or does not match the hash.
func (c *Client) VerifySecret(candidate string) bool {
	if c.secretHash == nil {
		return true
	}
	if candidate == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(*c.secretHash), []byte(candidate)) == nil
}

// SecretHash returns the stored bcrypt hash for confidential clients,
// or nil for public clients. Exposed primarily so infrastructure-layer
// repositories can persist the aggregate; application code should
// prefer VerifySecret / IsConfidential.
func (c *Client) SecretHash() *string {
	return c.secretHash
}

func fromSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out
}
