package authcode

// Nonce is a value object wrapping the OPTIONAL OIDC "nonce"
// authorization request parameter (OIDC Core 3.1.2.1). It is used to
// mitigate replay attacks: when present, its value must be echoed
// verbatim into the ID Token's "nonce" claim.
type Nonce struct {
	value string
}

// NewNonce wraps s as a Nonce. An empty string is a valid Nonce
// (representing "no nonce was requested"); see IsEmpty.
func NewNonce(s string) Nonce {
	return Nonce{value: s}
}

// String returns the underlying string representation of the Nonce.
// It returns "" when no nonce was requested.
func (n Nonce) String() string {
	return n.value
}

// IsEmpty reports whether no nonce value was requested.
func (n Nonce) IsEmpty() bool {
	return n.value == ""
}
