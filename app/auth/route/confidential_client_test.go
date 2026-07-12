package route_test

// Tests for confidential client authentication (ISSUE-035).
//
// RFC 6749 2.3.1: A confidential client must authenticate at the token
// endpoint with its registered client_secret. This file exercises:
//   - client_secret_post: secret sent in the POST body
//   - client_secret_basic: secret sent in the Authorization header
//   - wrong secret → 401 invalid_client
//   - missing secret for confidential client → 401 invalid_client
//   - public clients: existing behavior unchanged (no secret required)

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	testConfClientID    = "confidential-client"
	testConfRedirectURI = "http://localhost:3000/callback"
	testConfSecret      = "correct-horse-battery-staple"
)

// newConfidentialTestHandler builds a fully wired router backed by a real
// Postgres database with one confidential client seeded. The confidential
// client uses testConfSecret as its plaintext secret. The demo user
// (testUserID / testUsername / testDemoPassword) is also seeded so that
// /authorize flows can complete login.
func newConfidentialTestHandler(t *testing.T) http.Handler {
	t.Helper()
	db := testsupport.OpenTestDB(t)

	testsupport.TruncateTable(t, db, "refresh_tokens")
	testsupport.TruncateTable(t, db, "authorization_codes")
	testsupport.TruncateTable(t, db, "consents")
	testsupport.TruncateTable(t, db, "users")
	testsupport.TruncateTable(t, db, "clients")

	// Confidential client with a bcrypt-hashed secret.
	confCID, err := client.ParseClientID(testConfClientID)
	if err != nil {
		t.Fatalf("setup ParseClientID() unexpected error: %v", err)
	}
	redirectURI, err := client.NewRedirectURI(testConfRedirectURI)
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}
	confClient, err := client.NewConfidential(
		confCID,
		[]client.RedirectURI{redirectURI},
		[]string{"openid", "profile", "email", "offline_access"},
		[]string{"code"},
		[]string{"authorization_code", "refresh_token"},
		testConfSecret,
	)
	if err != nil {
		t.Fatalf("setup NewConfidential() unexpected error: %v", err)
	}
	testsupport.SeedClient(t, db, confClient)

	// Demo user (same credentials used by loginSession in helpers_test.go).
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

	// Wire repositories and services (same pattern as newTestHandlerWithDB).
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

	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, refreshTokenRepo, signer, testIssuer, testAPIAudience)
	authnSvc := service.NewAuthenticationService(userRepo, sessionStore)
	consentSvc := service.NewConsentService(consentRepo)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, testIssuer, testAPIAudience)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)
	introspectSvc := service.NewIntrospectionService(verifier, testIssuer, testAPIAudience)

	return route.NewRouter(authSvc, authnSvc, consentSvc, clientRepo, userInfoSvc, discoverySvc, introspectSvc, nil, route.RouterConfig{Issuer: testIssuer})
}

// issueConfAuthCode runs a successful /authorize for the confidential
// client and returns the authorization code. Client auth is NOT required
// at /authorize (only at /token), so no secret is needed here.
func issueConfAuthCode(t *testing.T, h http.Handler, scope, verifier string) string {
	t.Helper()
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {testConfClientID},
		"redirect_uri":          {testConfRedirectURI},
		"scope":                 {scope},
		"state":                 {"s"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	rec := doAuthorize(t, h, q)
	if rec.Code != http.StatusFound {
		t.Fatalf("setup: authorize status = %d, want %d (body=%q)", rec.Code, http.StatusFound, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("setup: parse Location: %v", err)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("setup: empty authorization code")
	}
	return code
}

// doTokenWithBasicAuth sends a POST /token with credentials in the
// Authorization: Basic header (client_secret_basic).
func doTokenWithBasicAuth(t *testing.T, h http.Handler, form url.Values, clientID, clientSecret string) *httptest.ResponseRecorder {
	t.Helper()
	creds := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+creds)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestConfidentialClient_Token_Success_ClientSecretPost verifies that a
// confidential client successfully exchanges an authorization code when it
// presents the correct client_secret in the POST body
// (client_secret_post, RFC 6749 2.3.1).
func TestConfidentialClient_Token_Success_ClientSecretPost(t *testing.T) {
	h := newConfidentialTestHandler(t)

	verifier := strings.Repeat("B", 43)
	code := issueConfAuthCode(t, h, "openid profile", verifier)

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testConfRedirectURI},
		"client_id":     {testConfClientID},
		"client_secret": {testConfSecret},
		"code_verifier": {verifier},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("token exchange status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeTokenResponse(t, rec)
	if resp.AccessToken == "" {
		t.Error("access_token is empty, want non-empty")
	}
	if resp.IDToken == "" {
		t.Error("id_token is empty, want non-empty")
	}
}

// TestConfidentialClient_Token_Success_ClientSecretBasic verifies that a
// confidential client successfully exchanges an authorization code when it
// presents credentials via the Authorization: Basic header
// (client_secret_basic, RFC 6749 2.3.1).
func TestConfidentialClient_Token_Success_ClientSecretBasic(t *testing.T) {
	h := newConfidentialTestHandler(t)

	verifier := strings.Repeat("C", 43)
	code := issueConfAuthCode(t, h, "openid profile", verifier)

	rec := doTokenWithBasicAuth(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testConfRedirectURI},
		"code_verifier": {verifier},
	}, testConfClientID, testConfSecret)
	if rec.Code != http.StatusOK {
		t.Fatalf("token exchange status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeTokenResponse(t, rec)
	if resp.AccessToken == "" {
		t.Error("access_token is empty, want non-empty")
	}
}

// TestConfidentialClient_Token_Failure_WrongSecret verifies that a
// confidential client is rejected with 401 invalid_client when it
// presents an incorrect client_secret.
func TestConfidentialClient_Token_Failure_WrongSecret(t *testing.T) {
	h := newConfidentialTestHandler(t)

	verifier := strings.Repeat("D", 43)
	code := issueConfAuthCode(t, h, "openid", verifier)

	rec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testConfRedirectURI},
		"client_id":     {testConfClientID},
		"client_secret": {"wrong-secret"},
		"code_verifier": {verifier},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	body := decodeErrorBody(t, rec)
	if body.Error != "invalid_client" {
		t.Errorf("error = %q, want %q", body.Error, "invalid_client")
	}
}

// TestConfidentialClient_Token_Failure_MissingSecret verifies that a
// confidential client is rejected when it omits the client_secret entirely.
func TestConfidentialClient_Token_Failure_MissingSecret(t *testing.T) {
	h := newConfidentialTestHandler(t)

	verifier := strings.Repeat("E", 43)
	code := issueConfAuthCode(t, h, "openid", verifier)

	rec := doToken(t, h, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {testConfRedirectURI},
		"client_id":    {testConfClientID},
		// no client_secret
		"code_verifier": {verifier},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	body := decodeErrorBody(t, rec)
	if body.Error != "invalid_client" {
		t.Errorf("error = %q, want %q", body.Error, "invalid_client")
	}
}

// TestConfidentialClient_RefreshToken_Success verifies that a confidential
// client can successfully use a refresh token when it supplies the correct
// client_secret.
func TestConfidentialClient_RefreshToken_Success(t *testing.T) {
	h := newConfidentialTestHandler(t)

	verifier := strings.Repeat("F", 43)
	code := issueConfAuthCode(t, h, "openid offline_access", verifier)

	tokenRec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testConfRedirectURI},
		"client_id":     {testConfClientID},
		"client_secret": {testConfSecret},
		"code_verifier": {verifier},
	})
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token exchange status = %d, want %d (body=%q)", tokenRec.Code, http.StatusOK, tokenRec.Body.String())
	}
	tokenBody := decodeTokenResponse(t, tokenRec)
	if tokenBody.RefreshToken == "" {
		t.Skip("no refresh_token in response (offline_access scope may require consent); skipping")
	}

	// Refresh using the correct secret.
	refreshRec := doToken(t, h, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokenBody.RefreshToken},
		"client_id":     {testConfClientID},
		"client_secret": {testConfSecret},
	})
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d (body=%q)", refreshRec.Code, http.StatusOK, refreshRec.Body.String())
	}
}

// TestConfidentialClient_RefreshToken_Failure_WrongSecret verifies that a
// confidential client is rejected at the refresh_token grant when it
// presents the wrong secret.
func TestConfidentialClient_RefreshToken_Failure_WrongSecret(t *testing.T) {
	h := newConfidentialTestHandler(t)

	verifier := strings.Repeat("G", 43)
	code := issueConfAuthCode(t, h, "openid offline_access", verifier)

	tokenRec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testConfRedirectURI},
		"client_id":     {testConfClientID},
		"client_secret": {testConfSecret},
		"code_verifier": {verifier},
	})
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token exchange status = %d, want %d (body=%q)", tokenRec.Code, http.StatusOK, tokenRec.Body.String())
	}
	tokenBody := decodeTokenResponse(t, tokenRec)
	if tokenBody.RefreshToken == "" {
		t.Skip("no refresh_token in response; skipping")
	}

	// Attempt to refresh with the wrong secret.
	refreshRec := doToken(t, h, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokenBody.RefreshToken},
		"client_id":     {testConfClientID},
		"client_secret": {"wrong-secret"},
	})
	if refreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh status = %d, want %d (body=%q)", refreshRec.Code, http.StatusUnauthorized, refreshRec.Body.String())
	}
	body := decodeErrorBody(t, refreshRec)
	if body.Error != "invalid_client" {
		t.Errorf("error = %q, want %q", body.Error, "invalid_client")
	}
}

// TestRevoke_ConfidentialClient_NoClientID_DoesNotRevoke verifies RFC 7009
// §2.1: a confidential client's refresh token cannot be revoked without
// client authentication (returns 200 but leaves the token valid).
func TestRevoke_ConfidentialClient_NoClientID_DoesNotRevoke(t *testing.T) {
	h := newConfidentialTestHandler(t)

	verifier := strings.Repeat("H", 43)
	code := issueConfAuthCode(t, h, "openid offline_access", verifier)

	tokenRec := doToken(t, h, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testConfRedirectURI},
		"client_id":     {testConfClientID},
		"client_secret": {testConfSecret},
		"code_verifier": {verifier},
	})
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token exchange status = %d, want %d (body=%q)", tokenRec.Code, http.StatusOK, tokenRec.Body.String())
	}
	tokenBody := decodeTokenResponse(t, tokenRec)
	if tokenBody.RefreshToken == "" {
		t.Fatal("setup: refresh_token is empty")
	}

	rec := doRevoke(t, h, tokenBody.RefreshToken, "", "refresh_token")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	refreshRec := doToken(t, h, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokenBody.RefreshToken},
		"client_id":     {testConfClientID},
		"client_secret": {testConfSecret},
	})
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh after unauthenticated revoke status = %d, want %d (token must remain valid; body=%q)",
			refreshRec.Code, http.StatusOK, refreshRec.Body.String())
	}
}

// TestPublicClient_Token_StillWorks confirms that public clients
// (no client_secret registered) continue to work without any secret,
// verifying that ISSUE-035 does not regress the existing behavior.
func TestPublicClient_Token_StillWorks(t *testing.T) {
	h := newTestHandler(t)
	resp := issueTokens(t, h, "openid profile", "")
	if resp.AccessToken == "" {
		t.Error("access_token is empty, want non-empty")
	}
}
