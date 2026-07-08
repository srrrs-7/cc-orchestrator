package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// ErrUnsupportedGrantType is returned by Token when grant_type is
// anything other than "authorization_code" -- the only grant this
// authorization server implements (RFC 6749 4.1.3).
var ErrUnsupportedGrantType = errors.New("service: unsupported grant type")

// responseTypeCode is the only OAuth response_type this authorization
// server accepts. Implicit ("token"/"id_token") and other flows are
// deliberately unsupported (see docs/plans/AUTH-001-plan.md "退けた代替案").
const responseTypeCode = "code"

// grantTypeAuthorizationCode is the only OAuth grant_type this
// authorization server's /token endpoint accepts.
const grantTypeAuthorizationCode = "authorization_code"

// AuthorizationService implements the OAuth 2.0 Authorization Code +
// PKCE grant's two core use cases: issuing an authorization code
// (Authorize) and exchanging it for an access token + ID Token
// (Token). It orchestrates the client, user, authcode and token
// bounded contexts but holds no business rule of its own; each
// aggregate/port enforces its own invariants.
type AuthorizationService struct {
	clients   client.Repository
	users     user.Repository
	authCodes authcode.Repository
	signer    token.Signer
	issuer    string

	// defaultUsername is the resource owner assigned to an /authorize
	// request when no login_hint is given (or the given login_hint
	// does not match a known user). See Authorize / resolveOwner for
	// the "where a real login/consent screen belongs" note.
	defaultUsername user.Username
}

// NewAuthorizationService builds an AuthorizationService.
func NewAuthorizationService(
	clients client.Repository,
	users user.Repository,
	authCodes authcode.Repository,
	signer token.Signer,
	issuer string,
	defaultUsername user.Username,
) *AuthorizationService {
	return &AuthorizationService{
		clients:         clients,
		users:           users,
		authCodes:       authCodes,
		signer:          signer,
		issuer:          issuer,
		defaultUsername: defaultUsername,
	}
}

// Authorize implements the authorization request of RFC 6749 4.1.1 /
// OIDC Core 3.1.2.1, restricted to response_type=code with mandatory
// PKCE (S256 only).
//
// Validation is deliberately ordered so that client_id and
// redirect_uri are confirmed *first*: RFC 6749 4.1.2.1 requires that
// if the client or the redirect_uri cannot be verified, the error
// MUST be shown to the resource owner directly rather than via
// redirect, since redirecting to an unverified/unregistered URI would
// make this endpoint an open redirector. Every error returned after
// that point is intentionally safe to report via a redirect to
// redirectURI (see route/response.go, which uses this exact ordering
// contract to decide "direct error" vs "redirect with error").
func (s *AuthorizationService) Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error) {
	cid, err := client.ParseClientID(req.ClientID)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}
	c, err := s.clients.FindByID(ctx, cid)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}

	redirectURI, err := client.NewRedirectURI(req.RedirectURI)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}
	if err := c.ValidateRedirectURI(redirectURI); err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}

	// --- client_id and redirect_uri are now verified. ---

	if req.ResponseType != responseTypeCode || !c.SupportsResponseType(responseTypeCode) {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", client.ErrUnsupportedResponseType)
	}

	scope, err := authcode.ParseScope(req.Scope)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}
	for _, v := range scope.Values() {
		if v == authcode.ScopeOpenID {
			continue
		}
		if !c.AllowsScope(v) {
			return AuthorizeResult{}, fmt.Errorf("service: authorize: scope %q not permitted: %w", v, authcode.ErrInvalidScope)
		}
	}

	method, err := authcode.ParseCodeChallengeMethod(req.CodeChallengeMethod)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}
	challenge, err := authcode.NewCodeChallenge(req.CodeChallenge, method)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}

	owner, err := s.resolveOwner(ctx, req.LoginHint)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}

	ac, err := authcode.New(
		authcode.NewClientID(c.ID().String()),
		authcode.NewUserID(owner.ID().String()),
		authcode.NewRedirectURI(redirectURI.String()),
		scope,
		authcode.NewNonce(req.Nonce),
		challenge,
	)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}
	if err := s.authCodes.Save(ctx, ac); err != nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: %w", err)
	}

	return AuthorizeResult{
		RedirectURI: redirectURI.String(),
		Code:        ac.Code().String(),
		State:       req.State,
	}, nil
}

// resolveOwner decides which User is the resource owner for an
// /authorize request.
//
// *** This is the simplification point where a real authorization
// server would show a login screen (to authenticate the resource
// owner) followed by a consent screen (to let them approve the
// requested scope), and only proceed once the owner has done both.
// This sample skips both: if loginHint names a known user it is used
// as-is (no password check), otherwise the seeded default user is
// used automatically. See README.md "認証/同意画面を差し込む箇所"
// for where to wire in a real implementation. ***
func (s *AuthorizationService) resolveOwner(ctx context.Context, loginHint string) (*user.User, error) {
	if loginHint != "" {
		username, err := user.NewUsername(loginHint)
		if err == nil {
			if u, findErr := s.users.FindByUsername(ctx, username); findErr == nil {
				return u, nil
			} else if !errors.Is(findErr, user.ErrNotFound) {
				return nil, findErr
			}
			// login_hint did not resolve to a known user: fall through
			// to the default user below rather than failing the
			// request, consistent with this sample's "no real login
			// UI" simplification.
		}
	}
	return s.users.FindByUsername(ctx, s.defaultUsername)
}

// Token implements the token request of RFC 6749 4.1.3, exchanging a
// previously issued authorization code (plus its PKCE code_verifier)
// for an access token and ID Token.
func (s *AuthorizationService) Token(ctx context.Context, req TokenRequest) (TokenResponse, error) {
	if req.GrantType != grantTypeAuthorizationCode {
		return TokenResponse{}, fmt.Errorf("service: token: %w", ErrUnsupportedGrantType)
	}

	cid, err := client.ParseClientID(req.ClientID)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}
	c, err := s.clients.FindByID(ctx, cid)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}
	if !c.SupportsGrantType(grantTypeAuthorizationCode) {
		return TokenResponse{}, fmt.Errorf("service: token: %w", client.ErrUnsupportedGrantType)
	}

	code, err := authcode.ParseCode(req.Code)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}
	ac, err := s.authCodes.FindByCode(ctx, code)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}

	if err := ac.Verify(req.CodeVerifier, authcode.NewRedirectURI(req.RedirectURI), authcode.NewClientID(c.ID().String())); err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}

	// Single-use enforcement is delegated to the repository's atomic
	// Consume, not to ac.Consume()+Save(): ac is a clone returned by
	// FindByCode above, so mutating it locally and saving it back
	// would be a read-modify-write against the store with no locking
	// in between (TOCTOU) -- two concurrent /token calls for the same
	// code could both pass Verify and both successfully "consume" and
	// save their own copy, each minting a valid token pair from a
	// single authorization code. Repository.Consume instead performs
	// the existence/expiry check and the deletion inside a single
	// critical section, so exactly one concurrent caller wins; every
	// other caller -- including this same goroutine on a genuine
	// replay after the code was already redeemed -- gets an error
	// here (mapped to invalid_grant by route/response.go).
	if err := s.authCodes.Consume(ctx, ac.Code()); err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}

	uid, err := user.ParseUserID(ac.UserID().String())
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}
	owner, err := s.users.FindByID(ctx, uid)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: %w", err)
	}

	scope := ac.Scope()

	// Access token audience design: this authorization server treats
	// its own /userinfo endpoint as the (only) resource server, so
	// the access token's "aud" is the issuer itself; UserInfoService
	// verifies that audience. A deployment adding real external
	// resource servers would need to revisit this (see
	// docs/plans/AUTH-001-plan.md "access token の aud 値の設計").
	accessClaims := token.NewAccessTokenClaims(s.issuer, owner.ID().String(), s.issuer, scope.String())
	accessToken, err := s.signer.Sign(accessClaims)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: sign access token: %w", err)
	}

	var name, email string
	if scope.Has("profile") {
		name = owner.Profile().Name()
	}
	if scope.Has("email") {
		email = owner.Profile().Email()
	}
	idClaims := token.NewIDTokenClaims(s.issuer, owner.ID().String(), c.ID().String(), ac.Nonce().String(), name, email)
	idToken, err := s.signer.Sign(idClaims)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: sign id token: %w", err)
	}

	return newTokenResponse(accessToken, idToken, scope.String()), nil
}
