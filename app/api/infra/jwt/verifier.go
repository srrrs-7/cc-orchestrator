// Package jwt provides a cross-service RS256 JWT verifier that fetches
// and caches public keys from a remote JWKS endpoint. Unlike app/auth's
// jwt package (which holds a single in-process key), this verifier is
// designed for API-side token validation: it fetches the auth server's
// published JWKS, caches keys by kid, and re-fetches on unknown kid or
// cache expiry to support key rotation without restart.
package jwt

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Sentinel errors returned by Verifier.Verify.
var (
	ErrInvalidToken     = errors.New("jwt: invalid token")
	ErrTokenExpired     = errors.New("jwt: token expired")
	ErrUnexpectedAlg    = errors.New("jwt: unexpected algorithm")
	ErrSignatureInvalid = errors.New("jwt: signature invalid")
	ErrInvalidClaims    = errors.New("jwt: invalid claims")
)

// Claims holds the JWT payload fields the API validates.
type Claims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  audClaim `json:"aud"`
	ExpiresAt int64    `json:"exp"`
	Scope     string   `json:"scope"`
}

// audClaim unmarshals a JWT "aud" field that may be either a JSON
// string or a JSON array of strings (RFC 7519 §4.1.3).
type audClaim []string

func (a *audClaim) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*a = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("aud: expected string or []string: %w", err)
	}
	*a = []string{s}
	return nil
}

func (a audClaim) contains(s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

// jwtHeader is the decoded JWT JOSE header.
type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// jwksKey is a single entry in a JWKS response.
type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwksResponse is the top-level JWKS document.
type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// cacheTTL is how long a fetched key set is considered fresh.
const cacheTTL = 5 * time.Minute

// Verifier validates RS256 JWTs by fetching and caching JWKS from a
// remote endpoint. It is safe for concurrent use.
type Verifier struct {
	jwksURL    string
	issuer     string
	audience   string
	httpClient *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// NewVerifier creates a Verifier that fetches JWKS from jwksURL and
// validates tokens against issuer (iss check) and audience (aud check).
// audience is the resource identifier this API was registered as
// (ISSUE-037); it is checked via "aud contains audience".
func NewVerifier(jwksURL, issuer, audience string) *Verifier {
	return &Verifier{
		jwksURL:    jwksURL,
		issuer:     issuer,
		audience:   audience,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		keys:       make(map[string]*rsa.PublicKey),
	}
}

// Verify parses tokenString as a compact RS256 JWT, fetches the signing
// key from the remote JWKS, and validates: alg=RS256, signature, exp,
// iss==issuer, aud contains audience (ISSUE-037), non-empty sub. It
// implements the route.TokenVerifier interface.
func (v *Verifier) Verify(ctx context.Context, tokenString string) error {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return ErrInvalidToken
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("%w: decode header", ErrInvalidToken)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return fmt.Errorf("%w: parse header", ErrInvalidToken)
	}
	// Strictly require RS256 to prevent algorithm-confusion attacks.
	if header.Alg != "RS256" {
		return fmt.Errorf("%w: alg=%q", ErrUnexpectedAlg, header.Alg)
	}

	key, err := v.getKey(ctx, header.Kid)
	if err != nil {
		return fmt.Errorf("%w: key lookup: %v", ErrInvalidToken, err)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("%w: decode signature", ErrInvalidToken)
	}
	signingInput := parts[0] + "." + parts[1]
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return ErrSignatureInvalid
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("%w: decode payload", ErrInvalidToken)
	}
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return fmt.Errorf("%w: parse claims", ErrInvalidToken)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return ErrTokenExpired
	}
	if claims.Issuer != v.issuer {
		return fmt.Errorf("%w: iss mismatch (got %q)", ErrInvalidClaims, claims.Issuer)
	}
	if !claims.Audience.contains(v.audience) {
		return fmt.Errorf("%w: aud does not contain expected audience", ErrInvalidClaims)
	}
	if claims.Subject == "" {
		return fmt.Errorf("%w: empty sub", ErrInvalidClaims)
	}

	return nil
}

// getKey returns the RSA public key for kid from the cache, fetching
// the JWKS when the cache is stale or the kid is unknown.
func (v *Verifier) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	fresh := time.Since(v.fetchedAt) < cacheTTL
	v.mu.RUnlock()

	if ok && fresh {
		return key, nil
	}

	if err := v.fetchJWKS(ctx); err != nil {
		// Prefer a stale cached key over a hard failure (transient network).
		v.mu.RLock()
		key, ok = v.keys[kid]
		v.mu.RUnlock()
		if ok {
			return key, nil
		}
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("kid %q not found in JWKS", kid)
	}
	return key, nil
}

// fetchJWKS retrieves the JWKS document from the configured URL and
// replaces the in-memory key cache atomically.
func (v *Verifier) fetchJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Alg != "RS256" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			return fmt.Errorf("parse key kid=%q: %w", k.Kid, err)
		}
		newKeys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = newKeys
	v.fetchedAt = time.Now()
	v.mu.Unlock()

	return nil
}

// parseRSAPublicKey decodes the base64url-encoded modulus n and public
// exponent e from a JWK and returns the corresponding *rsa.PublicKey.
func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)

	var eBig big.Int
	eBig.SetBytes(eBytes)
	e := int(eBig.Int64())
	if e == 0 {
		return nil, fmt.Errorf("public exponent is zero or overflows int")
	}

	return &rsa.PublicKey{N: n, E: e}, nil
}
