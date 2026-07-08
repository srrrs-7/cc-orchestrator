package token

// Verifier is a port (domain-declared interface) for verifying a
// compact JWT's signature and expiry and recovering its Claims. The
// concrete implementation (infra/jwt.Verifier) performs RS256
// verification and rejects any "alg" other than "RS256".
//
// Verify only checks the signature and the "exp" claim; it does not
// know the expected "iss"/"aud" for a given call site, so those
// checks are performed by the caller (see
// service.UserInfoService.UserInfo).
type Verifier interface {
	Verify(tokenString string) (Claims, error)
}
