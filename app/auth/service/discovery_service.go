package service

import (
	"context"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// DiscoveryService implements the OIDC Discovery use cases:
// publishing this authorization server's ProviderMetadata (OIDC
// Discovery 1.0 3) and JWK Set (RFC 7517).
type DiscoveryService struct {
	issuer string
	keys   token.KeyProvider
}

// NewDiscoveryService builds a DiscoveryService. issuer is the base
// URL every endpoint in ProviderMetadata is derived from.
//
// OIDC requires a production issuer to use https; this sample
// authorization server defaults issuer to http://localhost:8080 for
// local development (env ISSUER, see cmd/authz/main.go) and expects
// a production deployment to inject an https issuer explicitly (see
// README.md "issuer は本番 https 注入").
func NewDiscoveryService(issuer string, keys token.KeyProvider) *DiscoveryService {
	return &DiscoveryService{issuer: issuer, keys: keys}
}

// Metadata builds the /.well-known/openid-configuration response.
// Every field listed here is REQUIRED (issuer, authorization_endpoint,
// token_endpoint, jwks_uri, response_types_supported,
// subject_types_supported, id_token_signing_alg_values_supported) or
// explicitly supported by this authorization server's feature set
// (userinfo_endpoint, scopes_supported, claims_supported,
// code_challenge_methods_supported, grant_types_supported,
// token_endpoint_auth_methods_supported).
func (s *DiscoveryService) Metadata(_ context.Context) ProviderMetadata {
	return ProviderMetadata{
		Issuer:                            s.issuer,
		AuthorizationEndpoint:             s.issuer + "/authorize",
		TokenEndpoint:                     s.issuer + "/token",
		UserInfoEndpoint:                  s.issuer + "/userinfo",
		JWKSURI:                           s.issuer + "/.well-known/jwks.json",
		ResponseTypesSupported:            []string{"code"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256"},
		ScopesSupported:                   []string{"openid", "profile", "email"},
		ClaimsSupported:                   []string{"sub", "iss", "aud", "exp", "iat", "nonce", "name", "email"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		GrantTypesSupported:               []string{"authorization_code"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	}
}

// JWKS builds the /.well-known/jwks.json response.
func (s *DiscoveryService) JWKS(_ context.Context) JWKSet {
	return s.keys.JWKS()
}
