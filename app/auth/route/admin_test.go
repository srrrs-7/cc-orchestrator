package route_test

// Tests for the admin management API (ISSUE-039).
//
// POST /admin/clients — create public and confidential OAuth clients.
// POST /admin/users   — create users.
//
// All admin routes are protected by a static ADMIN_API_KEY that must be
// presented as either:
//   - Authorization: Bearer <key>
//   - X-Admin-Key: <key>
//
// Missing or wrong key yields 401; correct key yields 201 on success.
// Routes are not registered at all when adminSvc is nil (no ADMIN_API_KEY).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/auth/route"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

const testAdminKey = "test-admin-api-key"

// newAdminTestHandler builds a fully wired router with admin routes
// enabled, backed by a real Postgres test database.
func newAdminTestHandler(t *testing.T) http.Handler {
	t.Helper()
	db := testsupport.OpenTestDB(t)

	testsupport.TruncateTable(t, db, "refresh_tokens")
	testsupport.TruncateTable(t, db, "authorization_codes")
	testsupport.TruncateTable(t, db, "consents")
	testsupport.TruncateTable(t, db, "users")
	testsupport.TruncateTable(t, db, "clients")

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
	clientWriter := postgres.NewClientWriter(db)
	userWriter := postgres.NewUserWriter(db)
	sessionStore := memory.NewIdPSessionStore()

	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, refreshTokenRepo, signer, testIssuer, testAPIAudience)
	authnSvc := service.NewAuthenticationService(userRepo, sessionStore)
	consentSvc := service.NewConsentService(consentRepo)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, testIssuer, testAPIAudience)
	discoverySvc := service.NewDiscoveryService(testIssuer, keyProvider)
	introspectSvc := service.NewIntrospectionService(verifier, testIssuer, testAPIAudience)
	adminSvc := service.NewAdminService(clientWriter, userWriter, clientRepo, userRepo)

	return route.NewRouter(authSvc, authnSvc, consentSvc, clientRepo, userInfoSvc, discoverySvc, introspectSvc, adminSvc, route.RouterConfig{
		Issuer:      testIssuer,
		AdminAPIKey: testAdminKey,
	})
}

// doAdminCreateClient sends a POST /admin/clients with the given key
// and JSON body.
func doAdminCreateClient(t *testing.T, h http.Handler, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/clients", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// doAdminCreateUser sends a POST /admin/users with the given Bearer
// token and JSON body.
func doAdminCreateUser(t *testing.T, h http.Handler, bearerKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if bearerKey != "" {
		req.Header.Set("Authorization", "Bearer "+bearerKey)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Auth middleware tests
// ---------------------------------------------------------------------------

// TestAdmin_MissingKey verifies that omitting the key yields 401.
func TestAdmin_MissingKey(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "some-client",
		"redirect_uris":  []string{"https://example.com/callback"},
		"allowed_scopes": []string{"openid"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	rec := doAdminCreateClient(t, h, "", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// TestAdmin_WrongKey verifies that a wrong key yields 401.
func TestAdmin_WrongKey(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "some-client",
		"redirect_uris":  []string{"https://example.com/callback"},
		"allowed_scopes": []string{"openid"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	rec := doAdminCreateClient(t, h, "wrong-key", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// TestAdmin_NoAdminSvc verifies that admin routes are not registered
// when adminSvc is nil (no ADMIN_API_KEY), returning 404.
func TestAdmin_NoAdminSvc(t *testing.T) {
	h := newTestHandler(t) // no admin service

	raw, _ := json.Marshal(map[string]any{"client_id": "x"})
	req := httptest.NewRequest(http.MethodPost, "/admin/clients", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 or 405 (admin routes not registered)", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Client creation tests
// ---------------------------------------------------------------------------

// TestAdmin_CreatePublicClient_Success verifies that a public client
// is created and returns 201 with the correct body.
func TestAdmin_CreatePublicClient_Success(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "new-public-client",
		"redirect_uris":  []string{"https://app.example/callback"},
		"allowed_scopes": []string{"openid", "profile"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	rec := doAdminCreateClient(t, h, testAdminKey, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp struct {
		ClientID       string `json:"client_id"`
		IsConfidential bool   `json:"is_confidential"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
	if resp.ClientID != "new-public-client" {
		t.Errorf("client_id = %q, want %q", resp.ClientID, "new-public-client")
	}
	if resp.IsConfidential {
		t.Error("is_confidential = true, want false for public client")
	}
}

// TestAdmin_CreateConfidentialClient_Success verifies that a
// confidential client (with client_secret) is created and returns 201
// with is_confidential=true.
func TestAdmin_CreateConfidentialClient_Success(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "new-conf-client",
		"redirect_uris":  []string{"https://backend.example/callback"},
		"allowed_scopes": []string{"openid", "profile"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code", "refresh_token"},
		"client_secret":  "super-secret-value",
	}
	rec := doAdminCreateClient(t, h, testAdminKey, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp struct {
		ClientID       string `json:"client_id"`
		IsConfidential bool   `json:"is_confidential"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
	if resp.ClientID != "new-conf-client" {
		t.Errorf("client_id = %q, want %q", resp.ClientID, "new-conf-client")
	}
	if !resp.IsConfidential {
		t.Error("is_confidential = false, want true for confidential client")
	}
}

// TestAdmin_CreateClient_BearerToken verifies that the Bearer token
// auth method also works for the create-client route.
func TestAdmin_CreateClient_BearerToken(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "bearer-client",
		"redirect_uris":  []string{"https://example.com/cb"},
		"allowed_scopes": []string{"openid"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/clients", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

// TestAdmin_CreateClient_InvalidRedirectURI verifies that a malformed
// redirect_uri is rejected with 400.
func TestAdmin_CreateClient_InvalidRedirectURI(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "bad-uri-client",
		"redirect_uris":  []string{"not-a-uri"},
		"allowed_scopes": []string{"openid"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	rec := doAdminCreateClient(t, h, testAdminKey, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestAdmin_CreateClient_MissingClientID verifies that an empty
// client_id is rejected with 400.
func TestAdmin_CreateClient_MissingClientID(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "",
		"redirect_uris":  []string{"https://example.com/callback"},
		"allowed_scopes": []string{"openid"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	rec := doAdminCreateClient(t, h, testAdminKey, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestAdmin_CreateClient_DuplicateClientID_Upserts verifies that
// creating a client with an existing client_id upserts idempotently
// (returns 201 on both calls; current Writer.Save is ON CONFLICT DO UPDATE).
func TestAdmin_CreateClient_DuplicateClientID_Upserts(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "dup-client",
		"redirect_uris":  []string{"https://example.com/callback"},
		"allowed_scopes": []string{"openid"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	first := doAdminCreateClient(t, h, testAdminKey, body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d (body=%q)", first.Code, http.StatusCreated, first.Body.String())
	}

	second := doAdminCreateClient(t, h, testAdminKey, body)
	if second.Code != http.StatusCreated {
		t.Fatalf("duplicate create status = %d, want %d (body=%q)", second.Code, http.StatusCreated, second.Body.String())
	}
}

// TestAdmin_CreateClient_MalformedJSON verifies that invalid JSON
// bodies are rejected with 400.
func TestAdmin_CreateClient_MalformedJSON(t *testing.T) {
	h := newAdminTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/clients", bytes.NewReader([]byte("{not-json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// User creation tests
// ---------------------------------------------------------------------------

// TestAdmin_CreateUser_Success verifies that a user is created with
// status 201.
func TestAdmin_CreateUser_Success(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"user_id":  "admin-user-001",
		"username": "alice",
		"password": "correct-horse-battery-staple",
		"name":     "Alice",
		"email":    "alice@example.com",
	}
	rec := doAdminCreateUser(t, h, testAdminKey, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
	if resp.UserID != "admin-user-001" {
		t.Errorf("user_id = %q, want %q", resp.UserID, "admin-user-001")
	}
	if resp.Username != "alice" {
		t.Errorf("username = %q, want %q", resp.Username, "alice")
	}
}

// TestAdmin_CreateUser_MissingPassword verifies that omitting password
// yields 400.
func TestAdmin_CreateUser_MissingPassword(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"user_id":  "admin-user-002",
		"username": "bob",
		"name":     "Bob",
		"email":    "bob@example.com",
		// no password
	}
	rec := doAdminCreateUser(t, h, testAdminKey, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestAdmin_CreateUser_WrongKey verifies that 401 is returned for a
// user creation attempt with the wrong key.
func TestAdmin_CreateUser_WrongKey(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"user_id":  "admin-user-003",
		"username": "carol",
		"password": "pass",
		"name":     "Carol",
		"email":    "carol@example.com",
	}
	rec := doAdminCreateUser(t, h, "bad-key", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// List endpoints
// ---------------------------------------------------------------------------

func doAdminListUsers(t *testing.T, h http.Handler, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doAdminListClients(t *testing.T, h http.Handler, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/clients", nil)
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdmin_ListUsers_Empty(t *testing.T) {
	h := newAdminTestHandler(t)

	rec := doAdminListUsers(t, h, testAdminKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Users []struct {
			UserID   string `json:"user_id"`
			Username string `json:"username"`
			Name     string `json:"name"`
			Email    string `json:"email"`
		} `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
	if len(resp.Users) != 0 {
		t.Fatalf("users len = %d, want 0", len(resp.Users))
	}
}

func TestAdmin_ListUsers_AfterCreate(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"user_id":  "list-user-001",
		"username": "lister",
		"password": "correct-horse-battery-staple",
		"name":     "Lister",
		"email":    "lister@example.com",
	}
	createRec := doAdminCreateUser(t, h, testAdminKey, body)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	rec := doAdminListUsers(t, h, testAdminKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Users []struct {
			UserID   string `json:"user_id"`
			Username string `json:"username"`
			Name     string `json:"name"`
			Email    string `json:"email"`
		} `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
	if len(resp.Users) != 1 {
		t.Fatalf("users len = %d, want 1", len(resp.Users))
	}
	if resp.Users[0].UserID != "list-user-001" {
		t.Errorf("user_id = %q, want %q", resp.Users[0].UserID, "list-user-001")
	}
	if resp.Users[0].Email != "lister@example.com" {
		t.Errorf("email = %q, want %q", resp.Users[0].Email, "lister@example.com")
	}
}

func TestAdmin_ListClients_AfterCreate(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "list-client-001",
		"redirect_uris":  []string{"https://example.com/callback"},
		"allowed_scopes": []string{"openid", "profile"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	createRec := doAdminCreateClient(t, h, testAdminKey, body)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	rec := doAdminListClients(t, h, testAdminKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Clients []struct {
			ClientID       string   `json:"client_id"`
			RedirectURIs   []string `json:"redirect_uris"`
			AllowedScopes  []string `json:"allowed_scopes"`
			ResponseTypes  []string `json:"response_types"`
			GrantTypes     []string `json:"grant_types"`
			IsConfidential bool     `json:"is_confidential"`
		} `json:"clients"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
	if len(resp.Clients) != 1 {
		t.Fatalf("clients len = %d, want 1", len(resp.Clients))
	}
	if resp.Clients[0].ClientID != "list-client-001" {
		t.Errorf("client_id = %q, want %q", resp.Clients[0].ClientID, "list-client-001")
	}
	if len(resp.Clients[0].AllowedScopes) != 2 {
		t.Errorf("allowed_scopes = %v, want len 2", resp.Clients[0].AllowedScopes)
	}
}

func TestAdmin_ListUsers_MissingKey(t *testing.T) {
	h := newAdminTestHandler(t)

	rec := doAdminListUsers(t, h, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func doAdminGetUser(t *testing.T, h http.Handler, key, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/users/"+userID, nil)
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doAdminUpdateUser(t *testing.T, h http.Handler, key, userID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+userID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doAdminDeleteUser(t *testing.T, h http.Handler, key, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+userID, nil)
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdmin_UserCRUD(t *testing.T) {
	h := newAdminTestHandler(t)

	createBody := map[string]any{
		"user_id":  "crud-user-001",
		"username": "crud-user",
		"password": "correct-horse-battery-staple",
		"name":     "CRUD User",
		"email":    "crud@example.com",
	}
	if rec := doAdminCreateUser(t, h, testAdminKey, createBody); rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", rec.Code, http.StatusCreated)
	}

	getRec := doAdminGetUser(t, h, testAdminKey, "crud-user-001")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d (body=%q)", getRec.Code, http.StatusOK, getRec.Body.String())
	}

	updateRec := doAdminUpdateUser(t, h, testAdminKey, "crud-user-001", map[string]any{
		"username": "crud-user-updated",
		"name":     "CRUD User Updated",
		"email":    "crud-updated@example.com",
	})
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d (body=%q)", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	deleteRec := doAdminDeleteUser(t, h, testAdminKey, "crud-user-001")
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d (body=%q)", deleteRec.Code, http.StatusNoContent, deleteRec.Body.String())
	}

	missingRec := doAdminGetUser(t, h, testAdminKey, "crud-user-001")
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", missingRec.Code, http.StatusNotFound)
	}
}

func doAdminDeleteClient(t *testing.T, h http.Handler, key, clientID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/admin/clients/"+clientID, nil)
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdmin_DeleteClient(t *testing.T) {
	h := newAdminTestHandler(t)

	body := map[string]any{
		"client_id":      "delete-client-001",
		"redirect_uris":  []string{"https://example.com/callback"},
		"allowed_scopes": []string{"openid"},
		"response_types": []string{"code"},
		"grant_types":    []string{"authorization_code"},
	}
	if rec := doAdminCreateClient(t, h, testAdminKey, body); rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", rec.Code, http.StatusCreated)
	}

	deleteRec := doAdminDeleteClient(t, h, testAdminKey, "delete-client-001")
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	listRec := doAdminListClients(t, h, testAdminKey)
	var resp struct {
		Clients []struct {
			ClientID string `json:"client_id"`
		} `json:"clients"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, c := range resp.Clients {
		if c.ClientID == "delete-client-001" {
			t.Fatalf("deleted client still listed")
		}
	}
}
