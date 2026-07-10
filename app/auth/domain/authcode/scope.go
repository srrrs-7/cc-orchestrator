package authcode

import (
	"fmt"
	"strings"
)

// ScopeOpenID is the scope value that MUST be present in every OIDC
// authorization request (OIDC Core 3.1.2.1).
const ScopeOpenID = "openid"

// Scope is a value object representing a space-delimited set of OAuth
// 2.0 / OIDC scope values (RFC 6749 3.3).
type Scope struct {
	value  string
	values map[string]struct{}
}

// ParseScope splits s on whitespace into individual scope values and
// constructs a Scope. It returns ErrMissingOpenIDScope if "openid" is
// not among them, and ErrInvalidScope if s is empty or contains no
// scope values.
func ParseScope(s string) (Scope, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return Scope{}, fmt.Errorf("authcode: parse scope: %w", ErrInvalidScope)
	}

	values := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		values[f] = struct{}{}
	}
	if _, ok := values[ScopeOpenID]; !ok {
		return Scope{}, fmt.Errorf("authcode: parse scope: %w", ErrMissingOpenIDScope)
	}

	// Canonicalize the string representation (dedup + stable order is
	// not required by the spec; we simply rejoin the original field
	// order for a stable, human-readable String()).
	return Scope{value: strings.Join(fields, " "), values: values}, nil
}

// Has reports whether scope value s was requested.
func (s Scope) Has(value string) bool {
	_, ok := s.values[value]
	return ok
}

// Values returns the individual scope values as a slice.
func (s Scope) Values() []string {
	out := make([]string, 0, len(s.values))
	for v := range s.values {
		out = append(out, v)
	}
	return out
}

// String returns the space-delimited scope string.
func (s Scope) String() string {
	return s.value
}
