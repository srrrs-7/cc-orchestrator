// Package route_test exercises the presentation layer end-to-end via
// httptest, following the app/api/route/task_handler_test.go pattern.
//
// Test organisation (SPEC-011 build-tag split):
//
//   - This file (untagged): constants, types, testRSAKey, TestMain,
//     offline HTTP helpers (doToken, doUserInfo), decodeErrorBody, and
//     minimal test-local stubs for offline error-injection tests.
//
//   - helpers_integration_test.go (//go:build integration): newTestHandler
//     backed by a real Postgres DB (testsupport.OpenTestDB), plus
//     integration-only HTTP helpers (doAuthorize, issueAuthCode,
//     issueTokens, doRefreshToken, decodeTokenResponse, decodeJWTPayload,
//     tokenResponseBody, userInfoBody, jwtPayload, testClientID2).
//
// The offline stubs in this file are NOT a general-purpose in-memory
// store (that would re-introduce the deleted infra/memory). They are
// the narrowest possible implementation of the domain port interface
// that satisfies one specific error-injection scenario -- exactly the
// "test-local fake" pattern .claude/rules/testing.md endorses.
package route_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/route"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

const (
	testIssuer      = "https://issuer.example"
	testClientID    = "test-client"
	testRedirectURI = "http://localhost:3000/callback"
	testUsername    = "test-user"
	testUserID      = "user-sub-1"
	testUserName    = "Test User"
	testUserEmail   = "test-user@example.com"
)

// testRSAKey is generated once for the whole route_test package (RSA
// key generation is comparatively slow) and reused read-only to wire
// every test's fresh router.
var testRSAKey *rsa.PrivateKey

func TestMain(m *testing.M) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	testRSAKey = key
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Offline handler helpers (no DB required)
// ---------------------------------------------------------------------------

// newDiscoveryTestHandler builds a router for offline discovery-only
// tests. The discovery endpoints (/.well-known/openid-configuration and
// /.well-known/jwks.json) only consult discoverySvc, which only needs
// the issuer string and keyProvider. authSvc and userInfoSvc are wired
// with nil repository values; they must never be called in a
// discovery-only test (if they are, a nil-pointer panic will surface
// the oversight immediately).
func newDiscoveryTestHandler(t *testing.T) http.Handler {
	t.Helper()

	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("setup ComputeKeyID() unexpected error: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)
	keyProvider := jwt.NewKeyProvider(&testRSAKey.PublicKey, kid)

	username, err := user.NewUsername(testUsername)
	if err != nil {
		t.Fatalf("setup NewUsername() unexpected error: %v", err)
	}

	// nil repos: discovery endpoints never call authSvc or userInfoSvc
	// methods that access the repositories. If a test accidentally
	// calls a non-discovery endpoint, the nil dereference will
	// immediately surface the mistake.
	authSvc := service.NewAuthorizationService(nil, nil, nil, nil, signer, testIssuer, username)
	userInfoSvc := service.NewUserInfoService(nil, verifier, testIssuer)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)
	return route.NewRouter(authSvc, userInfoSvc, discoverySvc)
}

// newTokenErrorTestHandler builds a router for offline error-injection
// tests that exercise the /token error path without requiring a live DB.
// It wires:
//   - clientRepo: returns the demo test client for testClientID, and
//     ErrNotFound for every other ID.
//   - authCodeRepo: always returns ErrNotFound (simulates a code
//     "does-not-exist" scenario -- exactly what
//     TestToken_ErrorResponse_HasNoCacheHeaders exercises).
//   - userRepo / refreshTokenRepo: panic if called (they must not be
//     reached on the error paths this helper targets).
func newTokenErrorTestHandler(t *testing.T) http.Handler {
	t.Helper()

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

	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("setup ComputeKeyID() unexpected error: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)
	keyProvider := jwt.NewKeyProvider(&testRSAKey.PublicKey, kid)

	username, err := user.NewUsername(testUsername)
	if err != nil {
		t.Fatalf("setup NewUsername() unexpected error: %v", err)
	}

	clientRepo := &stubClientOnlyRepo{c: demoClient}
	authCodeRepo := alwaysNotFoundAuthCodeRepo{}

	authSvc := service.NewAuthorizationService(clientRepo, nil, authCodeRepo, nil, signer, testIssuer, username)
	userInfoSvc := service.NewUserInfoService(nil, verifier, testIssuer)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)
	return route.NewRouter(authSvc, userInfoSvc, discoverySvc)
}

// ---------------------------------------------------------------------------
// Minimal test-local stubs (NOT general-purpose in-memory stores)
// ---------------------------------------------------------------------------

// stubClientOnlyRepo is a minimal client.Repository stub that returns
// a single pre-seeded client for its ID, and ErrNotFound for
// everything else. It exists solely to satisfy the service layer's
// client lookup in error-injection tests where the interesting
// assertion is about what happens *after* the client is found.
type stubClientOnlyRepo struct{ c *client.Client }

var _ client.Repository = (*stubClientOnlyRepo)(nil)

func (r *stubClientOnlyRepo) FindByID(_ context.Context, id client.ClientID) (*client.Client, error) {
	if r.c != nil && id == r.c.ID() {
		return r.c, nil
	}
	return nil, fmt.Errorf("stubClientOnlyRepo: %w", client.ErrNotFound)
}

// alwaysNotFoundAuthCodeRepo is a minimal authcode.Repository stub
// where every lookup immediately returns ErrNotFound and every write
// panics. Used for error-injection tests that expect /token to return
// invalid_grant because the code does not exist.
type alwaysNotFoundAuthCodeRepo struct{}

var _ authcode.Repository = alwaysNotFoundAuthCodeRepo{}

func (alwaysNotFoundAuthCodeRepo) FindByCode(_ context.Context, _ authcode.Code) (*authcode.AuthorizationCode, error) {
	return nil, fmt.Errorf("alwaysNotFoundAuthCodeRepo: %w", authcode.ErrNotFound)
}

func (alwaysNotFoundAuthCodeRepo) Save(_ context.Context, _ *authcode.AuthorizationCode) error {
	panic("alwaysNotFoundAuthCodeRepo.Save must not be called in error-injection tests")
}

func (alwaysNotFoundAuthCodeRepo) Consume(_ context.Context, _ authcode.Code) error {
	panic("alwaysNotFoundAuthCodeRepo.Consume must not be called in error-injection tests")
}

// panicRefreshTokenRepo implements refreshtoken.Repository and panics
// on any call. Used to verify that a given error path never reaches
// the refresh token layer.
type panicRefreshTokenRepo struct{}

var _ refreshtoken.Repository = panicRefreshTokenRepo{}

func (panicRefreshTokenRepo) FindByTokenHash(_ context.Context, _ refreshtoken.TokenHash) (*refreshtoken.RefreshToken, error) {
	panic("panicRefreshTokenRepo.FindByTokenHash must not be called")
}

func (panicRefreshTokenRepo) Save(_ context.Context, _ *refreshtoken.RefreshToken) error {
	panic("panicRefreshTokenRepo.Save must not be called")
}

func (panicRefreshTokenRepo) Rotate(_ context.Context, _ refreshtoken.TokenHash, _ *refreshtoken.RefreshToken) error {
	panic("panicRefreshTokenRepo.Rotate must not be called")
}

func (panicRefreshTokenRepo) RevokeFamily(_ context.Context, _ refreshtoken.FamilyID) error {
	panic("panicRefreshTokenRepo.RevokeFamily must not be called")
}

// ---------------------------------------------------------------------------
// HTTP helper functions (shared by offline and integration tests)
// ---------------------------------------------------------------------------

func doToken(t *testing.T, h http.Handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doUserInfo(t *testing.T, h http.Handler, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/userinfo", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type errorBody struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var got errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode error response: %v (body=%q)", err, rec.Body.String())
	}
	return got
}
