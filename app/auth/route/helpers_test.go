// Package route_test exercises the presentation layer end-to-end via
// httptest, wiring a fresh in-memory repository set (with seeded demo
// client/user) and a test RSA key behind route.NewRouter for every
// test case, following the app/api/route/task_handler_test.go
// pattern.
package route_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
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

	// testClientID2 is a second registered client, distinct from
	// testClientID, used by refresh_token tests that need a
	// legitimately registered (but different) client to exercise
	// RFC 6749 §6 client binding (SPEC-006 R6): presenting a refresh
	// token issued to testClientID while authenticating as
	// testClientID2 must be rejected as invalid_grant, not
	// invalid_client/unsupported_grant_type -- so testClientID2 also
	// supports grant_type=refresh_token.
	testClientID2    = "test-client-2"
	testRedirectURI2 = "http://localhost:3000/callback2"
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

// newTestHandler builds a fresh, fully wired router backed by empty
// in-memory repositories seeded with one demo client and one demo
// user, so every test starts from independent, known state.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	clientRepo := memory.NewClientRepository()
	userRepo := memory.NewUserRepository()
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

	// A second, distinct client (also supporting refresh_token) used
	// by SPEC-006's client-binding tests (R6): see testClientID2's doc
	// comment above.
	cid2, err := client.ParseClientID(testClientID2)
	if err != nil {
		t.Fatalf("setup ParseClientID() unexpected error: %v", err)
	}
	redirectURI2, err := client.NewRedirectURI(testRedirectURI2)
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}
	otherClient := client.New(
		cid2,
		[]client.RedirectURI{redirectURI2},
		[]string{"openid", "profile", "email"},
		[]string{"code"},
		[]string{"authorization_code", "refresh_token"},
	)
	clientRepo.Seed(otherClient)

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

// pkceChallenge independently computes the RFC 7636 S256 transformation.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func doAuthorize(t *testing.T, h http.Handler, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/authorize?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

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

// issueAuthCode drives a successful /authorize request and returns
// the issued authorization code, for use as setup in /token-focused
// tests.
func issueAuthCode(t *testing.T, h http.Handler, challenge, scope, nonce string) string {
	t.Helper()

	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {scope},
		"state":                 {"setup-state"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if nonce != "" {
		q.Set("nonce", nonce)
	}

	rec := doAuthorize(t, h, q)
	if rec.Code != http.StatusFound {
		t.Fatalf("setup: authorize status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("setup: parse Location header: %v", err)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("setup: redirect code is empty, want non-empty authorization code")
	}
	return code
}

// issueTokens drives a full authorize -> token (grant_type=
// authorization_code) exchange using a fixed PKCE verifier, and
// returns the decoded token response -- including the freshly issued
// refresh_token, since testClientID supports grant_type=refresh_token
// (SPEC-006 R2). It fails the test (t.Fatalf) on any unexpected
// status, so callers can treat it as pure setup.
func issueTokens(t *testing.T, h http.Handler, scope, nonce string) tokenResponseBody {
	t.Helper()

	verifier := strings.Repeat("A", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifier), scope, nonce)

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifier},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: token exchange status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	return decodeTokenResponse(t, rec)
}

// doRefreshToken drives a POST /token request with
// grant_type=refresh_token (SPEC-006 R1). scope is only sent when
// non-empty, matching the optional scope parameter's semantics (RFC
// 6749 §6: omitting it means "use the scope originally granted").
func doRefreshToken(t *testing.T, h http.Handler, refreshToken, clientID, scope string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	if scope != "" {
		form.Set("scope", scope)
	}
	return doToken(t, h, form)
}

type tokenResponseBody struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

type errorBody struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type userInfoBody struct {
	Subject string `json:"sub"`
	Name    string `json:"name"`
	Email   string `json:"email"`
}

func decodeTokenResponse(t *testing.T, rec *httptest.ResponseRecorder) tokenResponseBody {
	t.Helper()
	var got tokenResponseBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode token response: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var got errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode error response: %v (body=%q)", err, rec.Body.String())
	}
	return got
}

// jwtPayload is a superset of the registered claims this test suite
// asserts on, decoded directly from a compact JWT's payload segment
// (independent of domain/token.Claims, so the test does not simply
// echo the production type back at itself).
type jwtPayload struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	Nonce     string `json:"nonce"`
}

func decodeJWTPayload(t *testing.T, tokenString string) jwtPayload {
	t.Helper()
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q is not a compact JWT (want 3 dot-separated segments)", tokenString)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims jwtPayload
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("unmarshal JWT payload: %v", err)
	}
	return claims
}
