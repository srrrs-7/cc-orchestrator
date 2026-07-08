package client

import "fmt"

// ClientID is a value object that uniquely identifies a registered
// OAuth Client. It is immutable; once created its value never changes.
type ClientID struct {
	value string
}

// ParseClientID validates and wraps an existing string as a ClientID.
// It rejects empty strings.
func ParseClientID(s string) (ClientID, error) {
	if s == "" {
		return ClientID{}, fmt.Errorf("client: parse client id: %w", ErrInvalidClientID)
	}
	return ClientID{value: s}, nil
}

// String returns the underlying string representation of the ClientID.
func (id ClientID) String() string {
	return id.value
}
