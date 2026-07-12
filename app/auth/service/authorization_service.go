package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// ErrUnsupportedGrantType is returned by Token when grant_type is
// anything other than "authorization_code" or "refresh_token" -- the
// only grants this authorization server implements (RFC 6749 4.1.3,
// SPEC-006 R1).
var ErrUnsupportedGrantType = errors.New("service: unsupported grant type")

// responseTypeCode is the only OAuth response_type this authorization
// server accepts. Implicit ("token"/"id_token") and other flows are
// deliberately unsupported (see docs/plans/AUTH-001-plan.md "退けた代替案").
const responseTypeCode = "code"

// grantTypeAuthorizationCode and grantTypeRefreshToken are the two
// OAuth grant_type values this authorization server's /token endpoint
// accepts (SPEC-006 adds grantTypeRefreshToken to AUTH-001's
// grantTypeAuthorizationCode).
const (
	grantTypeAuthorizationCode = "authorization_code"
	grantTypeRefreshToken      = "refresh_token"
)

// AuthorizationService implements the OAuth 2.0 Authorization Code +
// PKCE grant's two core use cases: issuing an authorization code
// (Authorize) and exchanging it for an access token + ID Token
// (Token). It orchestrates the client, user, authcode and token
// bounded contexts but holds no business rule of its own; each
// aggregate/port enforces its own invariants.
type AuthorizationService struct {
	clients       client.Repository
	users         user.Repository
	authCodes     authcode.Repository
	refreshTokens refreshtoken.Repository
	signer        token.Signer
	issuer        string
	apiAudience   string
}

// NewAuthorizationService builds an AuthorizationService.
// apiAudience is the resource identifier placed in access token "aud"
// claims (ISSUE-037). It identifies app/api as the intended recipient
// of the access token, distinct from the issuer's own UserInfo endpoint.
func NewAuthorizationService(
	clients client.Repository,
	users user.Repository,
	authCodes authcode.Repository,
	refreshTokens refreshtoken.Repository,
	signer token.Signer,
	issuer string,
	apiAudience string,
) *AuthorizationService {
	return &AuthorizationService{
		clients:       clients,
		users:         users,
		authCodes:     authCodes,
		refreshTokens: refreshTokens,
		signer:        signer,
		issuer:        issuer,
		apiAudience:   apiAudience,
	}
}

// ValidateAuthorize performs every /authorize check except resource owner
// resolution. It returns a verified AuthorizeResult (RedirectURI set)
// when client_id and redirect_uri are confirmed, so callers can safely
// redirect unauthenticated users to login without becoming an open
// redirector (ISSUE-031).
func (s *AuthorizationService) ValidateAuthorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error) {
	return s.validateAuthorize(ctx, req)
}

// Authorize implements the authorization request of RFC 6749 4.1.1 /
// OIDC Core 3.1.2.1 for an already-authenticated resource owner.
func (s *AuthorizationService) Authorize(ctx context.Context, req AuthorizeRequest, owner *user.User) (AuthorizeResult, error) {
	if owner == nil {
		return AuthorizeResult{}, fmt.Errorf("service: authorize: authenticated resource owner required")
	}
	verified, err := s.validateAuthorize(ctx, req)
	if err != nil {
		return verified, err
	}

	cid, err := client.ParseClientID(req.ClientID)
	if err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}
	scope, err := authcode.ParseScope(req.Scope)
	if err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}
	method, err := authcode.ParseCodeChallengeMethod(req.CodeChallengeMethod)
	if err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}
	challenge, err := authcode.NewCodeChallenge(req.CodeChallenge, method)
	if err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}

	ac, err := authcode.New(
		authcode.NewClientID(cid.String()),
		authcode.NewUserID(owner.ID().String()),
		authcode.NewRedirectURI(verified.RedirectURI),
		scope,
		authcode.NewNonce(req.Nonce),
		challenge,
		req.AuthTime,
	)
	if err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}
	if err := s.authCodes.Save(ctx, ac); err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}

	return AuthorizeResult{
		RedirectURI: verified.RedirectURI,
		Code:        ac.Code().String(),
		State:       req.State,
	}, nil
}

func (s *AuthorizationService) validateAuthorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error) {
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
	//
	// verified carries redirectURI.String() -- the exact string that
	// ValidateRedirectURI just confirmed matches one of the client's
	// registered redirect URIs (client.RedirectURI.String() returns its
	// value unchanged, so this is byte-for-byte the value validated
	// above, not a normalized/re-derived one). Every error return past
	// this point uses verified instead of the zero-value AuthorizeResult{}
	// used above, so route/authorize_handler.go's error path always
	// receives a redirect_uri that came from Authorize itself having
	// already confirmed it, rather than reaching for the raw request
	// value directly (see writeAuthorizeError in route/response.go,
	// gosec G710 rationale, ISSUE-004).
	verified := AuthorizeResult{RedirectURI: redirectURI.String()}

	if req.ResponseType != responseTypeCode || !c.SupportsResponseType(responseTypeCode) {
		return verified, fmt.Errorf("service: authorize: %w", client.ErrUnsupportedResponseType)
	}

	scope, err := authcode.ParseScope(req.Scope)
	if err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}
	for _, v := range scope.Values() {
		if v == authcode.ScopeOpenID {
			continue
		}
		if !c.AllowsScope(v) {
			return verified, fmt.Errorf("service: authorize: scope %q not permitted: %w", v, authcode.ErrInvalidScope)
		}
	}

	method, err := authcode.ParseCodeChallengeMethod(req.CodeChallengeMethod)
	if err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}
	if _, err := authcode.NewCodeChallenge(req.CodeChallenge, method); err != nil {
		return verified, fmt.Errorf("service: authorize: %w", err)
	}

	return verified, nil
}

// Token implements the token request of RFC 6749 4.1.3/6, dispatching
// on grant_type to one of this authorization server's two supported
// grants (SPEC-006 R1): exchanging a previously issued authorization
// code (authorizationCodeGrant) or redeeming a refresh token
// (refreshTokenGrant).
func (s *AuthorizationService) Token(ctx context.Context, req TokenRequest) (TokenResponse, error) {
	switch req.GrantType {
	case grantTypeAuthorizationCode:
		return s.authorizationCodeGrant(ctx, req)
	case grantTypeRefreshToken:
		return s.refreshTokenGrant(ctx, req)
	default:
		return TokenResponse{}, fmt.Errorf("service: token: %w", ErrUnsupportedGrantType)
	}
}

// authorizationCodeGrant implements grant_type=authorization_code
// (RFC 6749 4.1.3), exchanging a previously issued authorization code
// (plus its PKCE code_verifier) for an access token and ID Token.
// When the requesting client also supports grant_type=refresh_token,
// it additionally mints and persists a new refresh token (SPEC-006
// R2), returned once as plaintext in the response.
func (s *AuthorizationService) authorizationCodeGrant(ctx context.Context, req TokenRequest) (TokenResponse, error) {
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
	// Confidential client authentication (ISSUE-035, RFC 6749 2.3.1):
	// verify the presented client_secret against the stored bcrypt hash.
	// Public clients (IsConfidential() == false) skip this check.
	if c.IsConfidential() && !c.VerifySecret(req.ClientSecret) {
		return TokenResponse{}, fmt.Errorf("service: token: %w", client.ErrClientAuthFailed)
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

	// Access token audience: the resource identifier for app/api
	// (ISSUE-037). This separates API-bound tokens from UserInfo-bound
	// tokens, reducing blast radius on token leakage.
	accessClaims := token.NewAccessTokenClaims(s.issuer, owner.ID().String(), s.apiAudience, scope.String())
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
	atHash := token.ComputeAtHash(accessToken)
	idClaims := token.NewIDTokenClaims(s.issuer, owner.ID().String(), c.ID().String(), ac.Nonce().String(), name, email, ac.AuthTime(), atHash)
	idToken, err := s.signer.Sign(idClaims)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: token: sign id token: %w", err)
	}

	// SPEC-006 R2 + OIDC Core §11: a refresh_token-capable client
	// receives a refresh token only when the granted scope includes
	// offline_access (OIDC Core §11). Without offline_access the token
	// endpoint returns access/ID tokens only (TokenResponse.RefreshToken
	// stays empty/omitted). A client not registered for
	// grant_type=refresh_token never receives a refresh token.
	var refreshTokenPlaintext string
	if c.SupportsGrantType(grantTypeRefreshToken) && scope.Has(authcode.ScopeOfflineAccess) {
		rtScope, err := refreshtoken.ParseScope(scope.String())
		if err != nil {
			return TokenResponse{}, fmt.Errorf("service: token: %w", err)
		}
		rt, plaintext, err := refreshtoken.Issue(
			refreshtoken.NewClientID(c.ID().String()),
			refreshtoken.NewUserID(owner.ID().String()),
			rtScope,
			ac.AuthTime(),
		)
		if err != nil {
			return TokenResponse{}, fmt.Errorf("service: token: issue refresh token: %w", err)
		}
		if err := s.refreshTokens.Save(ctx, rt); err != nil {
			return TokenResponse{}, fmt.Errorf("service: token: save refresh token: %w", err)
		}
		refreshTokenPlaintext = plaintext.String()
	}

	return newTokenResponse(accessToken, idToken, scope.String(), refreshTokenPlaintext), nil
}

// minRefreshGrantDuration is the constant-time floor applied in
// refreshTokenGrant when FindByTokenHash returns ErrNotFound
// (ISSUE-019 item 2: timing side-channel mitigation). Without this
// floor, an attacker can distinguish "token hash unknown" (fast: one
// index miss) from "token hash found but valid" (slower: several more
// DB operations) purely by response latency. 10 ms is low enough not
// to noticeably affect legitimate clients, but high enough to mask
// the sub-millisecond difference between an index miss and a hit on a
// well-tuned Postgres host.
const minRefreshGrantDuration = 10 * time.Millisecond

// refreshTokenGrant implements grant_type=refresh_token (RFC 6749 6,
// SPEC-006 R1). It validates the presented refresh token (existence,
// reuse detection, client binding, scope narrowing), reissues an
// access token and ID Token, and atomically rotates the refresh token
// itself (R4): the caller receives a brand new refresh token and the
// one just redeemed becomes unusable. See
// docs/plans/SPEC-006-plan.md "service リフレッシュフロー" for the
// authoritative step-by-step contract this method follows.
func (s *AuthorizationService) refreshTokenGrant(ctx context.Context, req TokenRequest) (TokenResponse, error) {
	// 1. An empty refresh_token can never match a persisted token; treat
	// it the same as "unknown token" (invalid_grant) rather than
	// invalid_request, mirroring RFC 6749 6's error semantics.
	if req.RefreshToken == "" {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", refreshtoken.ErrNotFound)
	}

	// 2. Resolve and validate the requesting client.
	cid, err := client.ParseClientID(req.ClientID)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}
	c, err := s.clients.FindByID(ctx, cid)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}
	if !c.SupportsGrantType(grantTypeRefreshToken) {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", client.ErrUnsupportedGrantType)
	}
	// Confidential client authentication (ISSUE-035): same check as in
	// authorizationCodeGrant -- the grant_type=refresh_token flow also
	// requires the client to authenticate if it is confidential.
	if c.IsConfidential() && !c.VerifySecret(req.ClientSecret) {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", client.ErrClientAuthFailed)
	}

	// 3. Look the presented token up by its hash; consumed-but-unexpired
	// rows are returned on purpose (see refreshtoken.Repository.FindByTokenHash).
	// Record the wall-clock start just before the DB lookup so that the
	// ErrNotFound path below can apply a constant-time floor
	// (minRefreshGrantDuration) to mitigate timing side-channel attacks
	// (ISSUE-019 item 2): an index miss is faster than a hit + subsequent
	// operations, and we do not want that difference to be observable.
	lookupStart := time.Now()
	oldHash := refreshtoken.HashToken(req.RefreshToken)
	rt, err := s.refreshTokens.FindByTokenHash(ctx, oldHash)
	if err != nil {
		// Timing mitigation: pad the ErrNotFound path to at least
		// minRefreshGrantDuration so callers cannot distinguish a missing
		// token from a valid one by response latency alone.
		if elapsed := time.Since(lookupStart); elapsed < minRefreshGrantDuration {
			time.Sleep(minRefreshGrantDuration - elapsed)
		}
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}

	// 4. Reuse detection (RFC 9700 4.14, SPEC-006 R5): a refresh token
	// that was already rotated being presented again is a signal the
	// whole rotation family must be revoked before reporting
	// invalid_grant.
	if rt.Consumed() {
		if revokeErr := s.refreshTokens.RevokeFamily(ctx, rt.FamilyID()); revokeErr != nil {
			return TokenResponse{}, fmt.Errorf("service: refresh token: revoke family: %w", revokeErr)
		}
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", refreshtoken.ErrReused)
	}

	// 5. Client binding (RFC 6749 6, SPEC-006 R6).
	if err := rt.MatchesClient(refreshtoken.NewClientID(c.ID().String())); err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}

	// 6. Scope narrowing (RFC 6749 6, SPEC-006 R7): an empty req.Scope
	// keeps the token's current scope; a non-empty one must be a
	// subset (widening is rejected as invalid_scope).
	effectiveScope, err := rt.Scope().Narrow(req.Scope)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}

	// 7. Resolve the resource owner the token was issued for.
	uid, err := user.ParseUserID(rt.UserID().String())
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}
	owner, err := s.users.FindByID(ctx, uid)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}

	// 8. Reissue access token (aud=apiAudience, same audience design as
	// authorizationCodeGrant, ISSUE-037) and ID Token (aud=client_id,
	// fresh iat, no nonce -- OIDC Core 12.2, SPEC-006 R3). iss/sub unchanged.
	accessClaims := token.NewAccessTokenClaims(s.issuer, owner.ID().String(), s.apiAudience, effectiveScope.String())
	accessToken, err := s.signer.Sign(accessClaims)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: sign access token: %w", err)
	}

	var name, email string
	if effectiveScope.Has("profile") {
		name = owner.Profile().Name()
	}
	if effectiveScope.Has("email") {
		email = owner.Profile().Email()
	}
	rtAtHash := token.ComputeAtHash(accessToken)
	idClaims := token.NewIDTokenClaims(s.issuer, owner.ID().String(), c.ID().String(), "", name, email, rt.AuthTime(), rtAtHash)
	idToken, err := s.signer.Sign(idClaims)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: sign id token: %w", err)
	}

	// 9. Atomically rotate: consume oldHash and persist the new
	// RefreshToken in one critical section (refreshtoken.Repository.Rotate).
	// A concurrent/replayed loser observes ErrReused here too (not just
	// at step 4's pre-check) and must trigger the same family
	// revocation (RFC 9700 4.14; SPEC-006 plan §リスク notes this is
	// intentionally strict even for legitimate concurrent retries).
	newRT, newPlaintext, err := rt.Rotate(effectiveScope)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("service: refresh token: rotate: %w", err)
	}
	if err := s.refreshTokens.Rotate(ctx, oldHash, newRT); err != nil {
		if errors.Is(err, refreshtoken.ErrReused) {
			if revokeErr := s.refreshTokens.RevokeFamily(ctx, rt.FamilyID()); revokeErr != nil {
				return TokenResponse{}, fmt.Errorf("service: refresh token: revoke family: %w", revokeErr)
			}
		}
		return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)
	}

	return newTokenResponse(accessToken, idToken, effectiveScope.String(), newPlaintext.String()), nil
}
