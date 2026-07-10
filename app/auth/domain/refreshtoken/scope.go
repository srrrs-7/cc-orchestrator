package refreshtoken

import (
	"fmt"
	"strings"
)

// Scope is a value object representing a space-delimited set of OAuth
// 2.0 / OIDC scope values (RFC 6749 3.3) granted to a RefreshToken.
//
// Unlike domain/authcode.Scope, Scope here does NOT require "openid"
// to be present: refreshtoken is a standalone, cross-cutting
// persistence aggregate that must not depend on authcode/OIDC-specific
// invariants (SPEC-006 plan §リスク "refreshtoken.Scope は openid 必須に
// しない = 他 domain 非結合"). Only emptiness is rejected.
type Scope struct {
	value  string
	values map[string]struct{}
}

// ParseScope splits s on whitespace into individual scope values and
// constructs a Scope. It returns ErrInvalidScope if s is empty or
// contains no scope values.
func ParseScope(s string) (Scope, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return Scope{}, fmt.Errorf("refreshtoken: parse scope: %w", ErrInvalidScope)
	}

	values := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		values[f] = struct{}{}
	}
	return Scope{value: strings.Join(fields, " "), values: values}, nil
}

// Has reports whether scope value v is present.
func (s Scope) Has(v string) bool {
	_, ok := s.values[v]
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

// Narrow computes the effective scope for a refresh request (RFC 6749
// 6, SPEC-006 R7): an empty requested string keeps s unchanged
// (omitting scope means "use the scope originally granted"); a
// non-empty requested string MUST be a subset of s's values (equal is
// allowed) and becomes the new effective Scope; any requested value
// not present in s is rejected as ErrInvalidScope (widening is never
// permitted).
func (s Scope) Narrow(requested string) (Scope, error) {
	fields := strings.Fields(requested)
	if len(fields) == 0 {
		return s, nil
	}

	values := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if !s.Has(f) {
			return Scope{}, fmt.Errorf("refreshtoken: narrow scope: value %q not in original grant: %w", f, ErrInvalidScope)
		}
		values[f] = struct{}{}
	}
	return Scope{value: strings.Join(fields, " "), values: values}, nil
}
