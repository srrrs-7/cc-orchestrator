package service

import (
	"context"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// IntrospectionService implements RFC 7662 Token Introspection for JWT
// access tokens issued by this authorization server.
type IntrospectionService struct {
	verifier    token.Verifier
	issuer      string
	apiAudience string
}

// NewIntrospectionService builds an IntrospectionService.
// verifier verifies RS256 JWT signatures; issuer and apiAudience are
// used to validate the iss and aud claims of introspected access tokens.
func NewIntrospectionService(verifier token.Verifier, issuer, apiAudience string) *IntrospectionService {
	return &IntrospectionService{
		verifier:    verifier,
		issuer:      issuer,
		apiAudience: apiAudience,
	}
}

// Introspect validates a JWT access token and returns its introspection
// response (RFC 7662 §2.2). For invalid, expired, malformed, or
// unrecognized tokens it always returns active=false without error --
// the RFC explicitly requires this to avoid leaking token state to
// unauthenticated callers.
func (s *IntrospectionService) Introspect(_ context.Context, tokenString string) IntrospectionResponse {
	if tokenString == "" {
		return IntrospectionResponse{Active: false}
	}
	claims, err := s.verifier.Verify(tokenString)
	if err != nil {
		return IntrospectionResponse{Active: false}
	}
	// Verify this token was issued by this authorization server and
	// is addressed to the configured API resource server (ISSUE-037).
	if claims.Issuer != s.issuer || claims.Audience != s.apiAudience {
		return IntrospectionResponse{Active: false}
	}
	return IntrospectionResponse{
		Active:  true,
		Subject: claims.Subject,
		Exp:     claims.ExpiresAt,
		Scope:   claims.Scope,
	}
}
