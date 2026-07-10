package refreshtoken

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// familyIDByteLength is the amount of randomness (in bytes) used to
// generate a FamilyID. 32 bytes (256 bits) of entropy, base64url-
// encoded, is comfortably enough for an unguessable identifier that
// two independently issued rotation chains never collide on.
const familyIDByteLength = 32

// FamilyID is a value object identifying a refresh token rotation
// chain ("family"): every token produced by successive Rotate calls
// starting from a single Issue shares the same FamilyID. It is the
// unit of reuse-detection revocation (RevokeFamily, RFC 9700 4.14).
type FamilyID struct {
	value string
}

// NewFamilyID generates a new cryptographically random, opaque
// FamilyID.
func NewFamilyID() (FamilyID, error) {
	buf := make([]byte, familyIDByteLength)
	if _, err := rand.Read(buf); err != nil {
		return FamilyID{}, fmt.Errorf("refreshtoken: new family id: %w", err)
	}
	return FamilyID{value: base64.RawURLEncoding.EncodeToString(buf)}, nil
}

// ParseFamilyID wraps an existing string (e.g. one loaded from
// storage) as a FamilyID. It rejects empty strings.
func ParseFamilyID(s string) (FamilyID, error) {
	if s == "" {
		return FamilyID{}, fmt.Errorf("refreshtoken: parse family id: %w", ErrInvalidToken)
	}
	return FamilyID{value: s}, nil
}

// String returns the underlying string representation of the
// FamilyID.
func (f FamilyID) String() string {
	return f.value
}
