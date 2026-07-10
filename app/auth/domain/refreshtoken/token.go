package refreshtoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenByteLength is the amount of randomness (in bytes) used to
// generate a refresh Token. 32 bytes (256 bits) of entropy,
// base64url-encoded, comfortably exceeds what is needed for an
// unguessable, opaque, long-lived refresh token (RFC 6749 10.10).
const tokenByteLength = 32

// Token is a value object wrapping an opaque refresh token's plaintext
// value. Per SPEC-006 R8, the plaintext is never persisted: it is
// returned to the caller exactly once (at issuance or rotation) and
// the server only ever stores its Hash.
type Token struct {
	value string
}

// NewToken generates a new cryptographically random, opaque Token.
func NewToken() (Token, error) {
	buf := make([]byte, tokenByteLength)
	if _, err := rand.Read(buf); err != nil {
		return Token{}, fmt.Errorf("refreshtoken: new token: %w", err)
	}
	return Token{value: base64.RawURLEncoding.EncodeToString(buf)}, nil
}

// String returns the underlying opaque string representation of the
// Token (the value a client presents as refresh_token).
func (t Token) String() string {
	return t.value
}

// Hash returns the SHA-256 hash of the Token's plaintext, identical to
// calling HashToken(t.String()). This is the value actually persisted
// by Repository implementations (SPEC-006 R8).
func (t Token) Hash() TokenHash {
	return HashToken(t.value)
}

// TokenHash is a value object wrapping the SHA-256 hash (hex-encoded)
// of a refresh token's plaintext. It is comparable (a plain string
// underneath) so it can be used as a map key by infra/memory and as a
// lookup/primary key column by infra/postgres.
type TokenHash string

// HashToken computes the SHA-256 hash (hex-encoded) of plaintext. It
// is deterministic: identical plaintext always yields the identical
// TokenHash, which is what the persistence layer relies on to look a
// presented refresh_token up by its hash.
func HashToken(plaintext string) TokenHash {
	sum := sha256.Sum256([]byte(plaintext))
	return TokenHash(hex.EncodeToString(sum[:]))
}

// ParseTokenHash wraps an existing string (e.g. one loaded from
// storage) as a TokenHash. It rejects empty strings.
func ParseTokenHash(s string) (TokenHash, error) {
	if s == "" {
		return "", fmt.Errorf("refreshtoken: parse token hash: %w", ErrInvalidToken)
	}
	return TokenHash(s), nil
}

// String returns the underlying hex string representation of the
// TokenHash.
func (h TokenHash) String() string {
	return string(h)
}
