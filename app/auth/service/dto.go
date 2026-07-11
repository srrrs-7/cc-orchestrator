// Package service contains the application layer: use case
// orchestration that coordinates domain objects (client, user,
// authcode, token) to fulfill a single OAuth 2.0 / OIDC request,
// without holding business rules itself. Only plain DTOs cross this
// layer's boundary into the presentation layer (route); domain
// objects never leak upward.
package service

import "github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"

// AuthorizeRequest is the application-layer input for
// AuthorizationService.Authorize, built by route/authorize_handler.go
// from the /authorize query string.
type AuthorizeRequest struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	LoginHint           string
}

// AuthorizeResult is the application-layer output of
// AuthorizationService.Authorize: enough information for
// route/authorize_handler.go to build the 302 redirect back to the
// client (RFC 6749 4.1.2).
type AuthorizeResult struct {
	RedirectURI string
	Code        string
	State       string
}

// TokenRequest is the application-layer input for
// AuthorizationService.Token, built by route/token_handler.go from
// the /token form body. RefreshToken and Scope are used only by
// grant_type=refresh_token (SPEC-006 R1/R7); Code/RedirectURI/
// CodeVerifier are used only by grant_type=authorization_code.
type TokenRequest struct {
	GrantType    string
	Code         string
	RedirectURI  string
	ClientID     string
	CodeVerifier string
	RefreshToken string
	Scope        string
}

// TokenResponse is the JSON body returned for a successful /token
// request (RFC 6749 5.1, OIDC Core 3.1.3.3). RefreshToken is included
// (SPEC-006 R2/R4) whenever the client is registered for
// grant_type=refresh_token; it is omitted from the JSON body
// otherwise.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// newTokenResponse builds the TokenResponse returned from a
// successful token exchange. refreshToken is the plaintext refresh
// token to include (empty when the client does not support
// grant_type=refresh_token).
func newTokenResponse(accessToken, idToken, scope, refreshToken string) TokenResponse {
	return TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(token.AccessTokenTTL.Seconds()),
		IDToken:      idToken,
		Scope:        scope,
		RefreshToken: refreshToken,
	}
}

// UserInfoDTO is the JSON body returned from the /userinfo endpoint
// (OIDC Core 5.3.2). Subject ("sub") is always present; Name/Email
// are populated only when the access token's scope granted
// "profile"/"email" respectively.
type UserInfoDTO struct {
	Subject string `json:"sub"`
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
}

// ProviderMetadata is the JSON body returned from
// /.well-known/openid-configuration (OIDC Discovery 1.0 3).
type ProviderMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RevocationEndpoint                string   `json:"revocation_endpoint"`
	UserInfoEndpoint                  string   `json:"userinfo_endpoint"`
	EndSessionEndpoint                string   `json:"end_session_endpoint"`
	JWKSURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// JWKSet is the JSON body returned from /.well-known/jwks.json,
// re-exported from the token domain package's shape (RFC 7517 5) so
// that route does not need to import domain/token directly for this
// response type.
type JWKSet = token.JWKSet
