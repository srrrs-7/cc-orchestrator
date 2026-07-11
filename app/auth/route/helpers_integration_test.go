//go:build integration

// Integration test helpers that build a fully Postgres-backed HTTP
// handler. Separated into their own file so the build tag keeps them
// out of the default offline `make test` (SPEC-009 / SPEC-011).
package route_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/auth/route"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

const (
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

// newTestHandler builds a fully wired router backed by a real Postgres
// database (SPEC-011). It opens a test DB via testsupport.OpenTestDB,
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
// from newTestHandler so integration tests that need direct DB access
// (e.g. to execute a DELETE to simulate a disappeared user, see
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
	// testClientID2 doc comment in helpers_test.go).
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
	demoUser := user.New(uid, username, "irrelevant-demo-password", profile)
	testsupport.SeedUser(t, db, demoUser)

	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("setup ComputeKeyID() unexpected error: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)
	keyProvider := jwt.NewKeyProvider(&testRSAKey.PublicKey, kid)

	// SPEC-010 wiring: client/user → reader, authcode/refreshtoken →
	// writer. In test runs, reader == writer == db (single pool).
	clientRepo := postgres.NewClientRepository(db)
	userRepo := postgres.NewUserRepository(db)
	authCodeRepo := postgres.NewAuthCodeRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)

	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, refreshTokenRepo, signer, testIssuer, username)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, testIssuer)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)

	return route.NewRouter(authSvc, userInfoSvc, discoverySvc)
}
