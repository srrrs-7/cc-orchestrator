package token

import "errors"

// Sentinel errors returned by the token domain package. Callers
// should use errors.Is to branch on these, since they may be wrapped
// with additional context via fmt.Errorf("...: %w", err).
var (
	// ErrInvalidToken is returned when a presented string is not a
	// well-formed compact JWT (three base64url segments).
	ErrInvalidToken = errors.New("token: invalid token")

	// ErrTokenExpired is returned when a JWT's "exp" claim is in the
	// past.
	ErrTokenExpired = errors.New("token: token expired")

	// ErrSignatureInvalid is returned when a JWT's signature does not
	// verify against the authorization server's public key.
	ErrSignatureInvalid = errors.New("token: signature invalid")

	// ErrUnexpectedAlg is returned when a JWT's header "alg" is
	// anything other than "RS256". This authorization server only
	// ever issues RS256-signed tokens, so it rejects any other
	// algorithm outright at verification time -- including "none" --
	// to defend against algorithm-confusion attacks where an attacker
	// crafts a token with a downgraded or absent algorithm and expects
	// the verifier to accept it.
	ErrUnexpectedAlg = errors.New("token: unexpected alg")
)
