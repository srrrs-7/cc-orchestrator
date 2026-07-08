package token

// Signer is a port (domain-declared interface) for producing a
// signed, compact JWT from a Claims value. The concrete
// implementation (infra/jwt.Signer) performs RS256 signing.
type Signer interface {
	Sign(claims Claims) (string, error)
}
