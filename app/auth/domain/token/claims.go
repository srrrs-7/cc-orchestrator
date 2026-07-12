// Package token declares the ports (interfaces) this authorization
// server uses to issue and verify RS256-signed JWTs (access tokens
// and ID Tokens), plus the Claims/JWK value types those ports operate
// on. It has no dependency on any other layer or bounded context; the
// concrete RSA implementation lives in infra/jwt (dependency
// inversion).
package token

import (
	"crypto/sha256"
	"encoding/base64"
	"time"
)

// AccessTokenTTL and IDTokenTTL are the lifetimes applied by
// NewAccessTokenClaims / NewIDTokenClaims.
const (
	AccessTokenTTL = 1 * time.Hour
	IDTokenTTL     = 1 * time.Hour
)

// Claims models the JWT claim set used by both access tokens and ID
// Tokens issued by this authorization server. JSON tags match the
// registered claim names (RFC 7519 4.1, OIDC Core 2). Optional
// claims that were not requested/applicable are omitted from the
// JSON representation via omitempty.
type Claims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	Nonce     string `json:"nonce,omitempty"`
	AuthTime  int64  `json:"auth_time,omitempty"`
	AtHash    string `json:"at_hash,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
}

// NewAccessTokenClaims builds the Claims for an OAuth 2.0 JWT access
// token. audience is the API resource identifier (ISSUE-037): it
// identifies the resource server (app/api) the token is valid for,
// distinct from the issuer's own UserInfo endpoint. Callers pass the
// configured apiAudience value (env: API_AUDIENCE), not the issuer.
func NewAccessTokenClaims(issuer, subject, audience, scope string) Claims {
	now := time.Now()
	return Claims{
		Issuer:    issuer,
		Subject:   subject,
		Audience:  audience,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(AccessTokenTTL).Unix(),
		Scope:     scope,
	}
}

// NewIDTokenClaims builds the Claims for an OIDC ID Token. audience
// MUST be the requesting client's client_id (OIDC Core 2). nonce,
// name and email are included only when non-empty: nonce is echoed
// back only when the authorization request carried one (OIDC Core
// 3.1.2.1), and name/email are included only when the corresponding
// scope ("profile"/"email") was granted. authTime is the time the
// resource owner authenticated at the IdP (OIDC Core auth_time
// claim); zero value is omitted. atHash is the access token hash
// computed via ComputeAtHash (OIDC Core at_hash); empty is omitted.
func NewIDTokenClaims(issuer, subject, audience, nonce, name, email string, authTime time.Time, atHash string) Claims {
	now := time.Now()
	c := Claims{
		Issuer:    issuer,
		Subject:   subject,
		Audience:  audience,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(IDTokenTTL).Unix(),
		Nonce:     nonce,
		AtHash:    atHash,
		Name:      name,
		Email:     email,
	}
	if !authTime.IsZero() {
		c.AuthTime = authTime.Unix()
	}
	return c
}

// ComputeAtHash derives the at_hash claim value from an access token
// string as specified by OIDC Core §2: SHA-256 the token, take the
// left half of the digest, and base64url-encode it (no padding). The
// hash algorithm matches this server's signing algorithm (RS256 →
// SHA-256).
func ComputeAtHash(accessToken string) string {
	hash := sha256.Sum256([]byte(accessToken))
	return base64.RawURLEncoding.EncodeToString(hash[:16])
}

// ExpiresAtTime returns ExpiresAt converted to a time.Time.
func (c Claims) ExpiresAtTime() time.Time {
	return time.Unix(c.ExpiresAt, 0)
}
