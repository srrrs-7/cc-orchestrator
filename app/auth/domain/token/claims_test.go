package token_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// TestNewAccessTokenClaims covers traceability #5/#6: the claim set
// backing access tokens carries iss/sub/aud/exp/iat and the granted
// scope, with exp = iat + AccessTokenTTL.
func TestNewAccessTokenClaims(t *testing.T) {
	claims := token.NewAccessTokenClaims("https://issuer.example", "user-1", "https://issuer.example", "openid profile")

	if claims.Issuer != "https://issuer.example" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "https://issuer.example")
	}
	if claims.Subject != "user-1" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user-1")
	}
	if claims.Audience != "https://issuer.example" {
		t.Errorf("Audience = %q, want %q", claims.Audience, "https://issuer.example")
	}
	if claims.Scope != "openid profile" {
		t.Errorf("Scope = %q, want %q", claims.Scope, "openid profile")
	}
	if claims.IssuedAt == 0 {
		t.Error("IssuedAt is zero, want set")
	}
	wantExp := claims.IssuedAt + int64(token.AccessTokenTTL.Seconds())
	if claims.ExpiresAt != wantExp {
		t.Errorf("ExpiresAt = %d, want %d (IssuedAt + AccessTokenTTL)", claims.ExpiresAt, wantExp)
	}
}

// TestNewIDTokenClaims covers traceability #6: ID Token REQUIRED
// claims (iss/sub/aud/exp/iat), plus optional nonce/name/email
// reflected only when non-empty (OIDC Core 3.1.2.1 / omitempty JSON
// shape).
func TestNewIDTokenClaims(t *testing.T) {
	t.Run("required claims and nonce reflection", func(t *testing.T) {
		claims := token.NewIDTokenClaims("https://issuer.example", "user-1", "client-1", "nonce-xyz", "Demo User", "demo@example.com", time.Time{}, "")

		if claims.Issuer != "https://issuer.example" {
			t.Errorf("Issuer = %q, want %q", claims.Issuer, "https://issuer.example")
		}
		if claims.Subject != "user-1" {
			t.Errorf("Subject = %q, want %q", claims.Subject, "user-1")
		}
		if claims.Audience != "client-1" {
			t.Errorf("Audience = %q, want %q", claims.Audience, "client-1")
		}
		if claims.Nonce != "nonce-xyz" {
			t.Errorf("Nonce = %q, want %q", claims.Nonce, "nonce-xyz")
		}
		if claims.IssuedAt == 0 {
			t.Error("IssuedAt is zero, want set")
		}
		wantExp := claims.IssuedAt + int64(token.IDTokenTTL.Seconds())
		if claims.ExpiresAt != wantExp {
			t.Errorf("ExpiresAt = %d, want %d (IssuedAt + IDTokenTTL)", claims.ExpiresAt, wantExp)
		}

		raw, err := json.Marshal(claims)
		if err != nil {
			t.Fatalf("json.Marshal() unexpected error: %v", err)
		}
		if !strings.Contains(string(raw), `"nonce":"nonce-xyz"`) {
			t.Errorf("marshaled claims = %s, want to contain nonce", raw)
		}
	})

	t.Run("empty nonce is omitted from the JSON representation", func(t *testing.T) {
		claims := token.NewIDTokenClaims("https://issuer.example", "user-1", "client-1", "", "", "", time.Time{}, "")

		raw, err := json.Marshal(claims)
		if err != nil {
			t.Fatalf("json.Marshal() unexpected error: %v", err)
		}
		if strings.Contains(string(raw), `"nonce"`) {
			t.Errorf("marshaled claims = %s, want no nonce field when none was requested", raw)
		}
		if strings.Contains(string(raw), `"name"`) || strings.Contains(string(raw), `"email"`) {
			t.Errorf("marshaled claims = %s, want no name/email fields when profile/email scope was not granted", raw)
		}
	})
}

func TestClaims_ExpiresAtTime(t *testing.T) {
	claims := token.NewAccessTokenClaims("https://issuer.example", "user-1", "https://issuer.example", "openid")

	if claims.ExpiresAtTime().Unix() != claims.ExpiresAt {
		t.Errorf("ExpiresAtTime().Unix() = %d, want %d", claims.ExpiresAtTime().Unix(), claims.ExpiresAt)
	}
}
