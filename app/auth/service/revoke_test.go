package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

type fakeRefreshTokenRepo struct {
	tokens          map[refreshtoken.TokenHash]*refreshtoken.RefreshToken
	revokedFamilies []refreshtoken.FamilyID
}

func newFakeRefreshTokenRepo() *fakeRefreshTokenRepo {
	return &fakeRefreshTokenRepo{tokens: make(map[refreshtoken.TokenHash]*refreshtoken.RefreshToken)}
}

func (f *fakeRefreshTokenRepo) FindByTokenHash(_ context.Context, hash refreshtoken.TokenHash) (*refreshtoken.RefreshToken, error) {
	rt, ok := f.tokens[hash]
	if !ok {
		return nil, refreshtoken.ErrNotFound
	}
	return rt, nil
}

func (f *fakeRefreshTokenRepo) Save(_ context.Context, rt *refreshtoken.RefreshToken) error {
	f.tokens[rt.TokenHash()] = rt
	return nil
}

func (f *fakeRefreshTokenRepo) Rotate(_ context.Context, _ refreshtoken.TokenHash, _ *refreshtoken.RefreshToken) error {
	return nil
}

func (f *fakeRefreshTokenRepo) RevokeFamily(_ context.Context, familyID refreshtoken.FamilyID) error {
	f.revokedFamilies = append(f.revokedFamilies, familyID)
	return nil
}

type fakeClientRepo struct {
	clients map[string]*client.Client
}

func newFakeClientRepo(clients ...*client.Client) *fakeClientRepo {
	m := make(map[string]*client.Client, len(clients))
	for _, c := range clients {
		m[c.ID().String()] = c
	}
	return &fakeClientRepo{clients: m}
}

func (f *fakeClientRepo) FindByID(_ context.Context, id client.ClientID) (*client.Client, error) {
	c, ok := f.clients[id.String()]
	if !ok {
		return nil, client.ErrNotFound
	}
	return c, nil
}

func newRevokeTestService(t *testing.T, rtRepo refreshtoken.Repository, clientRepo client.Repository) *service.AuthorizationService {
	t.Helper()
	return service.NewAuthorizationService(clientRepo, nil, nil, rtRepo, nil, testIssuer, testAPIAudience)
}

func seedRefreshToken(t *testing.T, repo *fakeRefreshTokenRepo, clientID string) (plaintext string, familyID refreshtoken.FamilyID) {
	t.Helper()
	scope, err := refreshtoken.ParseScope("openid offline_access")
	if err != nil {
		t.Fatalf("ParseScope: %v", err)
	}
	rt, token, err := refreshtoken.Issue(refreshtoken.NewClientID(clientID), refreshtoken.NewUserID("user-1"), scope, time.Time{})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := repo.Save(context.Background(), rt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return token.String(), rt.FamilyID()
}

func newPublicTestClient(t *testing.T, id string) *client.Client {
	t.Helper()
	cid, err := client.ParseClientID(id)
	if err != nil {
		t.Fatalf("ParseClientID: %v", err)
	}
	uri, err := client.NewRedirectURI("https://example.com/callback")
	if err != nil {
		t.Fatalf("NewRedirectURI: %v", err)
	}
	return client.New(cid, []client.RedirectURI{uri}, []string{"openid"}, []string{"code"}, []string{"authorization_code", "refresh_token"})
}

func newConfidentialTestClient(t *testing.T, id, secret string) *client.Client {
	t.Helper()
	cid, err := client.ParseClientID(id)
	if err != nil {
		t.Fatalf("ParseClientID: %v", err)
	}
	uri, err := client.NewRedirectURI("https://example.com/callback")
	if err != nil {
		t.Fatalf("NewRedirectURI: %v", err)
	}
	c, err := client.NewConfidential(cid, []client.RedirectURI{uri}, []string{"openid"}, []string{"code"}, []string{"authorization_code", "refresh_token"}, secret)
	if err != nil {
		t.Fatalf("NewConfidential: %v", err)
	}
	return c
}

func TestRevoke_PublicClient_NoClientID_RevokesFamily(t *testing.T) {
	const clientID = "public-client"
	rtRepo := newFakeRefreshTokenRepo()
	tokenPlain, familyID := seedRefreshToken(t, rtRepo, clientID)
	svc := newRevokeTestService(t, rtRepo, newFakeClientRepo(newPublicTestClient(t, clientID)))

	if err := svc.Revoke(context.Background(), service.RevokeRequest{
		Token: tokenPlain,
	}); err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}
	if len(rtRepo.revokedFamilies) != 1 || rtRepo.revokedFamilies[0] != familyID {
		t.Errorf("revokedFamilies = %v, want [%v]", rtRepo.revokedFamilies, familyID)
	}
}

func TestRevoke_PublicClient_ClientMismatch_NoRevoke(t *testing.T) {
	const clientID = "public-client"
	rtRepo := newFakeRefreshTokenRepo()
	tokenPlain, familyID := seedRefreshToken(t, rtRepo, clientID)
	svc := newRevokeTestService(t, rtRepo, newFakeClientRepo(newPublicTestClient(t, clientID), newPublicTestClient(t, "other-client")))

	if err := svc.Revoke(context.Background(), service.RevokeRequest{
		Token:    tokenPlain,
		ClientID: "other-client",
	}); err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}
	if len(rtRepo.revokedFamilies) != 0 {
		t.Errorf("revokedFamilies = %v, want empty (client mismatch)", rtRepo.revokedFamilies)
	}
	_ = familyID
}

func TestRevoke_ConfidentialClient_ValidAuth_RevokesFamily(t *testing.T) {
	const (
		clientID = "conf-client"
		secret   = "correct-secret"
	)
	rtRepo := newFakeRefreshTokenRepo()
	tokenPlain, familyID := seedRefreshToken(t, rtRepo, clientID)
	svc := newRevokeTestService(t, rtRepo, newFakeClientRepo(newConfidentialTestClient(t, clientID, secret)))

	if err := svc.Revoke(context.Background(), service.RevokeRequest{
		Token:        tokenPlain,
		ClientID:     clientID,
		ClientSecret: secret,
	}); err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}
	if len(rtRepo.revokedFamilies) != 1 || rtRepo.revokedFamilies[0] != familyID {
		t.Errorf("revokedFamilies = %v, want [%v]", rtRepo.revokedFamilies, familyID)
	}
}

func TestRevoke_ConfidentialClient_NoClientID_NoRevoke(t *testing.T) {
	const (
		clientID = "conf-client"
		secret   = "correct-secret"
	)
	rtRepo := newFakeRefreshTokenRepo()
	tokenPlain, _ := seedRefreshToken(t, rtRepo, clientID)
	svc := newRevokeTestService(t, rtRepo, newFakeClientRepo(newConfidentialTestClient(t, clientID, secret)))

	if err := svc.Revoke(context.Background(), service.RevokeRequest{
		Token: tokenPlain,
	}); err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}
	if len(rtRepo.revokedFamilies) != 0 {
		t.Errorf("revokedFamilies = %v, want empty (confidential client must authenticate)", rtRepo.revokedFamilies)
	}
}

func TestRevoke_ConfidentialClient_WrongSecret_NoRevoke(t *testing.T) {
	const (
		clientID = "conf-client"
		secret   = "correct-secret"
	)
	rtRepo := newFakeRefreshTokenRepo()
	tokenPlain, _ := seedRefreshToken(t, rtRepo, clientID)
	svc := newRevokeTestService(t, rtRepo, newFakeClientRepo(newConfidentialTestClient(t, clientID, secret)))

	if err := svc.Revoke(context.Background(), service.RevokeRequest{
		Token:        tokenPlain,
		ClientID:     clientID,
		ClientSecret: "wrong-secret",
	}); err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}
	if len(rtRepo.revokedFamilies) != 0 {
		t.Errorf("revokedFamilies = %v, want empty (wrong secret)", rtRepo.revokedFamilies)
	}
}

func TestRevoke_UnknownToken_NoError(t *testing.T) {
	rtRepo := newFakeRefreshTokenRepo()
	svc := newRevokeTestService(t, rtRepo, newFakeClientRepo(newPublicTestClient(t, "public-client")))

	if err := svc.Revoke(context.Background(), service.RevokeRequest{
		Token:    "unknown-token",
		ClientID: "public-client",
	}); err != nil {
		t.Fatalf("Revoke() error = %v, want nil for unknown token", err)
	}
	if len(rtRepo.revokedFamilies) != 0 {
		t.Errorf("revokedFamilies = %v, want empty", rtRepo.revokedFamilies)
	}
}

func TestRevoke_EmptyToken_NoError(t *testing.T) {
	rtRepo := newFakeRefreshTokenRepo()
	svc := newRevokeTestService(t, rtRepo, newFakeClientRepo(newPublicTestClient(t, "public-client")))

	if err := svc.Revoke(context.Background(), service.RevokeRequest{}); err != nil {
		t.Fatalf("Revoke() error = %v, want nil for empty token", err)
	}
}
