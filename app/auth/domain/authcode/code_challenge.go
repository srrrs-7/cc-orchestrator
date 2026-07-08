package authcode

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// CodeChallengeMethod identifies the PKCE transformation applied to
// the code_verifier to produce a code_challenge (RFC 7636 4.2).
type CodeChallengeMethod struct {
	value string
}

// The two PKCE transformation methods defined by RFC 7636. Only
// CodeChallengeMethodS256 is accepted by this authorization server's
// /authorize endpoint; CodeChallengeMethodPlain exists so the type is
// complete and CodeChallenge.Verify can explicitly reject it with
// ErrUnsupportedChallengeMethod rather than silently mishandling it.
var (
	CodeChallengeMethodPlain = CodeChallengeMethod{value: "plain"}
	CodeChallengeMethodS256  = CodeChallengeMethod{value: "S256"}
)

// ParseCodeChallengeMethod validates and converts s into a
// CodeChallengeMethod. It returns ErrUnsupportedChallengeMethod if s
// does not match a known method.
func ParseCodeChallengeMethod(s string) (CodeChallengeMethod, error) {
	switch s {
	case CodeChallengeMethodPlain.value:
		return CodeChallengeMethodPlain, nil
	case CodeChallengeMethodS256.value:
		return CodeChallengeMethodS256, nil
	default:
		return CodeChallengeMethod{}, fmt.Errorf("authcode: parse code challenge method %q: %w", s, ErrUnsupportedChallengeMethod)
	}
}

// String returns the underlying string representation of the
// CodeChallengeMethod.
func (m CodeChallengeMethod) String() string {
	return m.value
}

// codeVerifierMinLen and codeVerifierMaxLen are the length bounds a
// code_verifier MUST satisfy per RFC 7636 4.1.
const (
	codeVerifierMinLen = 43
	codeVerifierMaxLen = 128
)

// CodeChallenge is a value object representing the PKCE
// code_challenge bound to an AuthorizationCode at authorization time
// (RFC 7636 4.3).
type CodeChallenge struct {
	challenge string
	method    CodeChallengeMethod
}

// NewCodeChallenge validates challenge/method and constructs a
// CodeChallenge. Only CodeChallengeMethodS256 is accepted; any other
// method (including "plain") is rejected with
// ErrUnsupportedChallengeMethod, per this authorization server's
// PKCE policy (S256-only).
func NewCodeChallenge(challenge string, method CodeChallengeMethod) (CodeChallenge, error) {
	if challenge == "" {
		return CodeChallenge{}, fmt.Errorf("authcode: new code challenge: %w", ErrInvalidCodeVerifier)
	}
	if method != CodeChallengeMethodS256 {
		return CodeChallenge{}, fmt.Errorf("authcode: new code challenge: %w", ErrUnsupportedChallengeMethod)
	}
	return CodeChallenge{challenge: challenge, method: method}, nil
}

// Method returns the PKCE transformation method bound to this
// CodeChallenge.
func (c CodeChallenge) Method() CodeChallengeMethod {
	return c.method
}

// Verify reports whether codeVerifier, once transformed using the
// bound method, reproduces this CodeChallenge's challenge value (RFC
// 7636 4.6). It first validates codeVerifier's length and character
// set (RFC 7636 4.1: 43-128 characters from the "unreserved" ABNF
// production), returning ErrInvalidCodeVerifier if that check fails.
// A mismatched transformation result yields ErrPKCEVerificationFailed.
func (c CodeChallenge) Verify(codeVerifier string) error {
	if err := validateCodeVerifier(codeVerifier); err != nil {
		return err
	}

	switch c.method {
	case CodeChallengeMethodS256:
		sum := sha256.Sum256([]byte(codeVerifier))
		computed := base64.RawURLEncoding.EncodeToString(sum[:])
		if computed != c.challenge {
			return ErrPKCEVerificationFailed
		}
		return nil
	default:
		// Reachable only if a CodeChallenge were constructed by means
		// other than NewCodeChallenge; kept for defense in depth.
		return ErrUnsupportedChallengeMethod
	}
}

// validateCodeVerifier enforces RFC 7636 4.1: a code_verifier is a
// high-entropy cryptographic random string using the unreserved
// characters [A-Z] / [a-z] / [0-9] / "-" / "." / "_" / "~", with a
// minimum length of 43 and a maximum length of 128 characters.
func validateCodeVerifier(codeVerifier string) error {
	n := len(codeVerifier)
	if n < codeVerifierMinLen || n > codeVerifierMaxLen {
		return ErrInvalidCodeVerifier
	}
	for _, r := range codeVerifier {
		if !isUnreserved(r) {
			return ErrInvalidCodeVerifier
		}
	}
	return nil
}

const unreservedExtra = "-._~"

func isUnreserved(r rune) bool {
	switch {
	case r >= 'A' && r <= 'Z':
		return true
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	default:
		return strings.ContainsRune(unreservedExtra, r)
	}
}
