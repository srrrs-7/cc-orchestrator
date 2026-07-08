// Package client contains the Client aggregate and its supporting
// domain types (value objects, repository interface and domain
// errors). This package has no dependency on any other layer, and it
// does not depend on any other bounded context (user / authcode /
// token): cross-aggregate references elsewhere in this system are
// expressed as plain IDs/values, never as a direct reference to a
// *Client.
package client

// Client is the aggregate root representing a registered OAuth 2.0
// client application. It is deliberately modeled as a "public
// client" (RFC 6749 2.1): this sample authorization server's primary
// flow is Authorization Code + PKCE, and it does not authenticate
// clients with a client_secret (token_endpoint_auth_methods_supported
// = ["none"]).
type Client struct {
	id            ClientID
	redirectURIs  []RedirectURI
	allowedScopes map[string]struct{}
	responseTypes map[string]struct{}
	grantTypes    map[string]struct{}
}

// New is the factory for registering a brand new Client.
func New(id ClientID, redirectURIs []RedirectURI, allowedScopes, responseTypes, grantTypes []string) *Client {
	return &Client{
		id:            id,
		redirectURIs:  append([]RedirectURI(nil), redirectURIs...),
		allowedScopes: toSet(allowedScopes),
		responseTypes: toSet(responseTypes),
		grantTypes:    toSet(grantTypes),
	}
}

// Reconstruct rebuilds a Client from already-validated persisted
// state. It is intended to be used exclusively by infrastructure-layer
// repository implementations when loading a Client from storage.
func Reconstruct(id ClientID, redirectURIs []RedirectURI, allowedScopes, responseTypes, grantTypes []string) *Client {
	return New(id, redirectURIs, allowedScopes, responseTypes, grantTypes)
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

func fromSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out
}
