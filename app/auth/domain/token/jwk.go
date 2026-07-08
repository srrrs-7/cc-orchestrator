package token

// JWK is a single JSON Web Key as defined by RFC 7517, restricted to
// the fields needed to publish an RSA public signing key.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKSet is a JSON Web Key Set (RFC 7517 5).
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// KeyProvider is a port (domain-declared interface) for publishing
// the authorization server's public signing key(s) via the JWKS
// endpoint. The concrete implementation (infra/jwt.KeyProvider)
// derives the JWK from an in-memory RSA public key.
type KeyProvider interface {
	PublicJWK() JWK
	JWKS() JWKSet
}
