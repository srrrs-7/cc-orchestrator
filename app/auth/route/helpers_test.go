// Package route_test exercises the presentation layer end-to-end via
// httptest, following the app/api/route/task_handler_test.go pattern.
//
// As of SPEC-013, this is a single, untagged test package: every
// DB-backed test in this package runs against a real Postgres test
// database (infra/postgres/testsupport.OpenTestDB, DB_NAME defaults to
// "auth_test", never the "auth" database used by `make up`) as part of
// the default `make test` / `make check`. There is no longer an
// `integration` build tag splitting this package's helpers in two --
// this file now holds both the router/HTTP helpers that always talk to
// a real DB (newTestHandler, newTestHandlerWithDB, doAuthorize,
// issueAuthCode, issueTokens, doRefreshToken, ...) and the couple of
// handlers below that deliberately do not wire a real repository.
//
// Those exceptions are NOT an in-memory store standing in for
// Postgres (SPEC-013 R2 forbids that): they wire nil repository values
// because the endpoints under test never reach a repository at all.
//
//   - newDiscoveryTestHandler: the discovery/JWKS endpoints, and the
//     iss/aud checks UserInfoService performs before ever touching
//     userRepo, never call the client/user repositories. If they ever
//     did, a nil-pointer panic would immediately surface the mistake
//     -- this is proof that the exercised code path never touches
//     persistence, not a DB substitute (SPEC-013 R2 example 2).
package route_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
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
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/auth/route"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

const (
	testIssuer       = "https://issuer.example"
	testClientID     = "test-client"
	testRedirectURI  = "http://localhost:3000/callback"
	testUsername     = "test-user"
	testUserID       = "user-sub-1"
	testUserName     = "Test User"
	testUserEmail    = "test-user@example.com"
	testDemoPassword = "test-password-123"
)

// testClientID2 is a second registered client, distinct from
// testClientID, used by refresh_token tests that need a legitimately
// registered (but different) client to exercise RFC 6749 §6 client
// binding (SPEC-006 R6): presenting a refresh token issued to
// testClientID while authenticating as testClientID2 must be rejected
// as invalid_grant, not invalid_client/unsupported_grant_type -- so
// testClientID2 also supports grant_type=refresh_token.
const (
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

// ---------------------------------------------------------------------------
// Test handlers
// ---------------------------------------------------------------------------

// newDiscoveryTestHandler builds a router for discovery-only tests.
// The discovery endpoints (/.well-known/openid-configuration and
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

	sessionStore := memory.NewIdPSessionStore()
	authnSvc := service.NewAuthenticationService(nil, sessionStore)
	consentSvc := service.NewConsentService(nil)
	authSvc := service.NewAuthorizationService(nil, nil, nil, nil, signer, testIssuer)
	userInfoSvc := service.NewUserInfoService(nil, verifier, testIssuer)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)
	_ = username
	return route.NewRouter(authSvc, authnSvc, consentSvc, nil, userInfoSvc, discoverySvc, route.RouterConfig{Issuer: testIssuer})
}

// newTokenErrorTestHandler builds a router for /token error-injection
// tests via newTestHandler (real Postgres test DB, SPEC-013): the
// seeded demo client (testClientID) exists, and the
// authorization_codes table starts truncated/empty, so submitting a
// code that was never issued (e.g. "does-not-exist") reproduces the
// same invalid_grant path a hand-written authcode stub previously
// simulated -- no additional fixture is required.
func newTokenErrorTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return newTestHandler(t)
}

// newTestHandler builds a fully wired router backed by a real Postgres
// database (SPEC-013). It opens a test DB via testsupport.OpenTestDB,
// truncates all four aggregate tables so each test starts from a clean
// slate, seeds both demo clients (testClientID / testClientID2) and the
// demo user (testUserID) via testsupport helpers, and wires real
// postgres.New*Repository implementations -- identical to what
// cmd/authz/main.go's setupPersistence does in production.
//
// SPEC-010: client/user are wired to the reader pool; authcode/
// refreshtoken are wired to the writer pool. In test runs, a single
// DB_* is set (no DB_READER_*), so reader == writer == db.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	db := testsupport.OpenTestDB(t)
	return newTestHandlerWithDB(t, db)
}

// newTestHandlerWithDB builds a fully wired router backed by the
// provided *sql.DB. It truncates and re-seeds all tables so each
// caller starts from a known, deterministic state. Exposed separately
// from newTestHandler so tests that need direct DB access (e.g. to
// execute a DELETE to simulate a disappeared user, see
// token_user_not_found_test.go) can share the same pool handle.
func newTestHandlerWithDB(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()

	// Truncate in dependency order: refresh_tokens and
	// authorization_codes reference users and clients via
	// application-level FKs (stored as plain text IDs; there are no
	// SQL-level FK constraints, so any truncation order works), but
	// starting with token tables is the safest conventional order.
	testsupport.TruncateTable(t, db, "refresh_tokens")
	testsupport.TruncateTable(t, db, "authorization_codes")
	testsupport.TruncateTable(t, db, "consents")
	testsupport.TruncateTable(t, db, "users")
	testsupport.TruncateTable(t, db, "clients")

	// Seed primary demo client.
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
	testsupport.SeedClient(t, db, demoClient)

	// Seed secondary demo client (used by client-binding tests, see
	// testClientID2's doc comment above).
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
	testsupport.SeedClient(t, db, otherClient)

	// Seed demo user.
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
	demoUser, err := user.New(uid, username, testDemoPassword, profile)
	if err != nil {
		t.Fatalf("setup user.New() unexpected error: %v", err)
	}
	testsupport.SeedUser(t, db, demoUser)

	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("setup ComputeKeyID() unexpected error: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)
	keyProvider := jwt.NewKeyProvider(&testRSAKey.PublicKey, kid)

	clientRepo := postgres.NewClientRepository(db)
	userRepo := postgres.NewUserRepository(db)
	authCodeRepo := postgres.NewAuthCodeRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	consentRepo := postgres.NewConsentRepository(db)
	sessionStore := memory.NewIdPSessionStore()

	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, refreshTokenRepo, signer, testIssuer)
	authnSvc := service.NewAuthenticationService(userRepo, sessionStore)
	consentSvc := service.NewConsentService(consentRepo)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, testIssuer)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)

	return route.NewRouter(authSvc, authnSvc, consentSvc, clientRepo, userInfoSvc, discoverySvc, route.RouterConfig{Issuer: testIssuer})
}

// ---------------------------------------------------------------------------
// HTTP helper functions
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

func doAuthorize(t *testing.T, h http.Handler, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	session := loginSession(t, h)
	return completeAuthorizeWithSession(t, h, query, session)
}

func completeAuthorizeWithSession(t *testing.T, h http.Handler, query url.Values, session *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := doAuthorizeWithSession(t, h, query, session)
	if rec.Code == http.StatusFound && strings.HasSuffix(rec.Header().Get("Location"), "/consent") {
		acceptConsent(t, h, session, rec)
		rec = doAuthorizeWithSession(t, h, query, session)
	}
	return rec
}

func acceptConsent(t *testing.T, h http.Handler, session *http.Cookie, authorizeRec *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/consent", strings.NewReader("action=accept"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)
	if pending := cookieFromResponse(authorizeRec, "idp_pending"); pending != nil {
		req.AddCookie(pending)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("consent accept status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
}

func cookieFromResponse(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func doAuthorizeWithSession(t *testing.T, h http.Handler, query url.Values, session *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/authorize?"+query.Encode(), nil)
	if session != nil {
		req.AddCookie(session)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func loginSession(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{
		"username": {testUsername},
		"password": {testDemoPassword},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "idp_session" {
			return c
		}
	}
	t.Fatal("login did not set idp_session cookie")
	return nil
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

// pkceChallenge independently computes the RFC 7636 S256 transformation.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
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

type tokenResponseBody struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
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
