package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

const (
	testIssuer      = "https://issuer.example"
	testAPIAudience = "https://api.example/api"
)

var testRSAKey *rsa.PrivateKey

func TestMain(m *testing.M) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	testRSAKey = key
	m.Run()
}

func newIntrospectionService(t *testing.T) *service.IntrospectionService {
	t.Helper()
	verifier := jwt.NewVerifier(&testRSAKey.PublicKey)
	return service.NewIntrospectionService(verifier, testIssuer, testAPIAudience)
}

func signAccessToken(t *testing.T, claims token.Claims) string {
	t.Helper()
	kid, err := jwt.ComputeKeyID(&testRSAKey.PublicKey)
	if err != nil {
		t.Fatalf("ComputeKeyID: %v", err)
	}
	signer := jwt.NewSigner(testRSAKey, kid)
	tokenString, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return tokenString
}

func validAccessClaims() token.Claims {
	return token.NewAccessTokenClaims(testIssuer, "user-1", testAPIAudience, "openid profile")
}

func TestIntrospectionService_ValidAccessToken_Active(t *testing.T) {
	svc := newIntrospectionService(t)
	tokenString := signAccessToken(t, validAccessClaims())

	got := svc.Introspect(context.Background(), tokenString)
	if !got.Active {
		t.Fatal("active = false, want true for a valid access token")
	}
	if got.Subject != "user-1" {
		t.Errorf("sub = %q, want %q", got.Subject, "user-1")
	}
	if got.Exp == 0 {
		t.Error("exp is zero, want non-zero")
	}
	if got.Scope != "openid profile" {
		t.Errorf("scope = %q, want %q", got.Scope, "openid profile")
	}
}

func TestIntrospectionService_EmptyToken_Inactive(t *testing.T) {
	svc := newIntrospectionService(t)
	got := svc.Introspect(context.Background(), "")
	if got.Active {
		t.Error("active = true, want false for empty token")
	}
}

func TestIntrospectionService_GarbageToken_Inactive(t *testing.T) {
	svc := newIntrospectionService(t)
	got := svc.Introspect(context.Background(), "not.a.jwt")
	if got.Active {
		t.Error("active = true, want false for garbage token")
	}
}

func TestIntrospectionService_WrongIssuer_Inactive(t *testing.T) {
	svc := newIntrospectionService(t)
	claims := validAccessClaims()
	claims.Issuer = "https://evil.example"
	tokenString := signAccessToken(t, claims)

	got := svc.Introspect(context.Background(), tokenString)
	if got.Active {
		t.Error("active = true, want false when iss does not match configured issuer")
	}
}

func TestIntrospectionService_WrongAudience_Inactive(t *testing.T) {
	svc := newIntrospectionService(t)
	claims := validAccessClaims()
	claims.Audience = testClientAudience
	tokenString := signAccessToken(t, claims)

	got := svc.Introspect(context.Background(), tokenString)
	if got.Active {
		t.Error("active = true, want false when aud does not match apiAudience")
	}
}

func TestIntrospectionService_ExpiredToken_Inactive(t *testing.T) {
	svc := newIntrospectionService(t)
	claims := validAccessClaims()
	claims.ExpiresAt = time.Now().Add(-1 * time.Hour).Unix()
	tokenString := signAccessToken(t, claims)

	got := svc.Introspect(context.Background(), tokenString)
	if got.Active {
		t.Error("active = true, want false for expired token")
	}
}

// testClientAudience mimics an ID token audience (client_id) rather than
// the API resource audience introspection expects.
const testClientAudience = "test-client"
