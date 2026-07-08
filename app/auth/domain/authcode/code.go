package authcode

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// codeByteLength is the amount of randomness (in bytes) used to
// generate an authorization Code. 32 bytes (256 bits) of entropy,
// base64url-encoded, comfortably exceeds what is needed for an
// unguessable, opaque, single-use authorization code (RFC 6749
// 10.10).
const codeByteLength = 32

// Code is a value object wrapping an opaque, single-use authorization
// code. Per RFC 6749 4.1.2, the value carries no meaning of its own;
// it is merely an unguessable reference the authorization server uses
// to look up the AuthorizationCode aggregate.
type Code struct {
	value string
}

// NewCode generates a new cryptographically random, opaque Code.
func NewCode() (Code, error) {
	buf := make([]byte, codeByteLength)
	if _, err := rand.Read(buf); err != nil {
		return Code{}, fmt.Errorf("authcode: new code: %w", err)
	}
	return Code{value: base64.RawURLEncoding.EncodeToString(buf)}, nil
}

// ParseCode wraps an existing string (e.g. one presented by a client
// at the token endpoint) as a Code. It rejects empty strings.
func ParseCode(s string) (Code, error) {
	if s == "" {
		return Code{}, fmt.Errorf("authcode: parse code: %w", ErrNotFound)
	}
	return Code{value: s}, nil
}

// String returns the underlying string representation of the Code.
func (c Code) String() string {
	return c.value
}
