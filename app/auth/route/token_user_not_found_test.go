// Regression coverage for ISSUE-019 課題3: when a grant (refresh_token
// / authorization_code) resolves to a valid, unexpired token/code but
// the resource owner it names has since disappeared from the user
// repository (e.g. deleted after the grant was issued), /token must
// respond with invalid_grant (HTTP 400) rather than the previous
// generic server_error (HTTP 500). See route/response.go's
// tokenErrorCode (the errors.Is(err, user.ErrNotFound) case, shared by
// both grants) and
// docs/issues/20260710-019-auth-refresh-token-deferred-hardening.md
// 課題3 for the full history.
//
// app/auth's own user.Repository has no delete operation (this sample
// never deletes users), so this file wires a small test-only
// removable user repository -- exactly enough surface to reproduce
// "the grant is still valid but its owner is gone".
package route_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/route"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

// removableUserRepository is a minimal, test-only user.Repository that
// additionally supports removing a previously seeded user, used to
// simulate "the resource owner existed when the grant was issued but
// has since been deleted" -- a scenario infra/memory.UserRepository
// cannot produce because it has no delete operation.
type removableUserRepository struct {
	mu   sync.RWMutex
	byID map[user.UserID]*user.User
}

var _ user.Repository = (*removableUserRepository)(nil)

func newRemovableUserRepository() *removableUserRepository {
	return &removableUserRepository{byID: make(map[user.UserID]*user.User)}
}

func (r *removableUserRepository) Seed(u *user.User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[u.ID()] = u
}

// Remove deletes the user with the given id, so a subsequent
// FindByID/FindByUsername observes user.ErrNotFound -- as if the user
// had been deleted after a still-valid grant naming it was issued.
func (r *removableUserRepository) Remove(id user.UserID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
}

func (r *removableUserRepository) FindByID(ctx context.Context, id user.UserID) (*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("removableUserRepository: find by id: %w", user.ErrNotFound)
	}
	return u, nil
}

func (r *removableUserRepository) FindByUsername(ctx context.Context, username user.Username) (*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.byID {
		if u.Username() == username {
			return u, nil
		}
	}
	return nil, fmt.Errorf("removableUserRepository: find by username: %w", user.ErrNotFound)
}

// newTestHandlerWithRemovableUser mirrors newTestHandler's wiring
// (helpers_test.go) -- same demo client/user, same test RSA key -- but
// takes an already-constructed removableUserRepository instead of
// infra/memory's UserRepository, so a caller can delete the seeded
// user mid-test.
func newTestHandlerWithRemovableUser(t *testing.T, userRepo *removableUserRepository) http.Handler {
	t.Helper()

	clientRepo := memory.NewClientRepository()
	authCodeRepo := memory.NewAuthCodeRepository()
	refreshTokenRepo := memory.NewRefreshTokenRepository()

	cid, err := client.ParseClientID(testClientID)
	if err != nil {
		t.Fatalf("setup ParseClientID() unexpected error: %v", err)
	}
	redirectURI, err := client.NewRedirectURI(testRedirectURI)
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}
	demoClient := client.New(
		cid,
		[]client.RedirectURI{redirectURI},
		[]string{"openid", "profile", "email"},
		[]string{"code"},
		[]string{"authorization_code", "refresh_token"},
	)
	clientRepo.Seed(demoClient)

	uid, err := user.ParseUserID(testUserID)
	if err != nil {
		t.Fatalf("setup ParseUserID() unexpected error: %v", err)
	}
	username, err := user.NewUsername(testUsername)
	if err != nil {
		t.Fatalf("setup NewUsername() unexpected error: %v", err)
	}
	profile, err := user.NewProfile(testUserName, testUserEmail)
	if err != nil {
		t.Fatalf("setup NewProfile() unexpected error: %v", err)
	}
	demoUser := user.New(uid, username, "irrelevant-demo-password", profile)
	userRepo.Seed(demoUser)

	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("setup ComputeKeyID() unexpected error: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)
	keyProvider := jwt.NewKeyProvider(&testRSAKey.PublicKey, kid)

	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, refreshTokenRepo, signer, testIssuer, username)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, testIssuer)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)

	return route.NewRouter(authSvc, userInfoSvc, discoverySvc)
}

// TestRefreshToken_UserNotFound_InvalidGrant covers ISSUE-019 課題3's
// refresh_token path: a refresh token that is itself valid (not
// expired/consumed/reused, correctly bound to the requesting client)
// but whose owner has disappeared from the user repository must be
// rejected as invalid_grant (400), not the previous generic
// server_error (500).
func TestRefreshToken_UserNotFound_InvalidGrant(t *testing.T) {
	userRepo := newRemovableUserRepository()
	h := newTestHandlerWithRemovableUser(t, userRepo)

	// Issue a full token set (including a refresh token) while the
	// resource owner still exists.
	orig := issueTokens(t, h, "openid", "")

	// Simulate the owner disappearing after the grant was issued.
	uid, err := user.ParseUserID(testUserID)
	if err != nil {
		t.Fatalf("setup ParseUserID() unexpected error: %v", err)
	}
	userRepo.Remove(uid)

	rec := doRefreshToken(t, h, orig.RefreshToken, testClientID, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	got := decodeErrorBody(t, rec)
	if got.Error != "invalid_grant" {
		t.Errorf("error = %q, want %q", got.Error, "invalid_grant")
	}

	// The description must not leak that the failure was specifically
	// "user not found": it must read like any other invalid_grant
	// cause, never mentioning the user (ISSUE-019 課題3: no
	// user-existence oracle via /token).
	if strings.Contains(strings.ToLower(got.ErrorDescription), "user") {
		t.Errorf("error_description = %q, must not mention \"user\" (would leak user-existence information)", got.ErrorDescription)
	}
	if want := "the authorization grant is invalid, expired, or already used"; got.ErrorDescription != want {
		t.Errorf("error_description = %q, want %q", got.ErrorDescription, want)
	}
}

// TestToken_AuthorizationCode_UserNotFound_InvalidGrant covers
// ISSUE-019 課題3's authorization_code path (the same tokenErrorCode
// fix covers both grants, sharing the errors.Is(err, user.ErrNotFound)
// case in route/response.go): an authorization code that is itself
// valid (correct PKCE, not expired/consumed) but whose owner has
// disappeared from the user repository must be rejected as
// invalid_grant (400), not server_error (500).
func TestToken_AuthorizationCode_UserNotFound_InvalidGrant(t *testing.T) {
	userRepo := newRemovableUserRepository()
	h := newTestHandlerWithRemovableUser(t, userRepo)

	verifier := strings.Repeat("A", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifier), "openid", "")

	// Simulate the owner disappearing after the code was issued but
	// before it is exchanged.
	uid, err := user.ParseUserID(testUserID)
	if err != nil {
		t.Fatalf("setup ParseUserID() unexpected error: %v", err)
	}
	userRepo.Remove(uid)

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifier},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	got := decodeErrorBody(t, rec)
	if got.Error != "invalid_grant" {
		t.Errorf("error = %q, want %q", got.Error, "invalid_grant")
	}

	if strings.Contains(strings.ToLower(got.ErrorDescription), "user") {
		t.Errorf("error_description = %q, must not mention \"user\" (would leak user-existence information)", got.ErrorDescription)
	}
	if want := "the authorization grant is invalid, expired, or already used"; got.ErrorDescription != want {
		t.Errorf("error_description = %q, want %q", got.ErrorDescription, want)
	}
}
