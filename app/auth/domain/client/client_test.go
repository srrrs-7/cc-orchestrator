package client_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
)

func newTestClient(t *testing.T) *client.Client {
	t.Helper()

	id, err := client.ParseClientID("demo-client")
	if err != nil {
		t.Fatalf("setup ParseClientID() unexpected error: %v", err)
	}
	redirectURI, err := client.NewRedirectURI("http://localhost:3000/callback")
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}

	return client.New(
		id,
		[]client.RedirectURI{redirectURI},
		[]string{"openid", "profile", "email"},
		[]string{"code"},
		[]string{"authorization_code", "refresh_token"},
	)
}

func TestClient_ValidateRedirectURI(t *testing.T) {
	c := newTestClient(t)

	t.Run("registered redirect_uri succeeds", func(t *testing.T) {
		registered, err := client.NewRedirectURI("http://localhost:3000/callback")
		if err != nil {
			t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
		}
		if err := c.ValidateRedirectURI(registered); err != nil {
			t.Fatalf("ValidateRedirectURI() unexpected error: %v", err)
		}
	})

	t.Run("unregistered redirect_uri is rejected", func(t *testing.T) {
		other, err := client.NewRedirectURI("http://localhost:3000/other")
		if err != nil {
			t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
		}
		err = c.ValidateRedirectURI(other)
		if !errors.Is(err, client.ErrRedirectURIMismatch) {
			t.Fatalf("ValidateRedirectURI() error = %v, want wrapping %v", err, client.ErrRedirectURIMismatch)
		}
	})
}

func TestClient_SupportsResponseType(t *testing.T) {
	c := newTestClient(t)

	if !c.SupportsResponseType("code") {
		t.Error("SupportsResponseType(\"code\") = false, want true")
	}
	if c.SupportsResponseType("token") {
		t.Error("SupportsResponseType(\"token\") = true, want false (implicit grant unsupported)")
	}
}

func TestClient_SupportsGrantType(t *testing.T) {
	c := newTestClient(t)

	if !c.SupportsGrantType("authorization_code") {
		t.Error("SupportsGrantType(\"authorization_code\") = false, want true")
	}
	if !c.SupportsGrantType("refresh_token") {
		t.Error("SupportsGrantType(\"refresh_token\") = false, want true")
	}
}

func TestClient_AllowsScope(t *testing.T) {
	c := newTestClient(t)

	if !c.AllowsScope("profile") {
		t.Error("AllowsScope(\"profile\") = false, want true")
	}
	if c.AllowsScope("admin") {
		t.Error("AllowsScope(\"admin\") = true, want false")
	}
}

// TestClient_RedirectURIs_IsACopy verifies that the slice returned by
// RedirectURIs is independent of the aggregate's internal state:
// mutating the returned slice must not affect subsequent calls.
func TestClient_RedirectURIs_IsACopy(t *testing.T) {
	c := newTestClient(t)

	got := c.RedirectURIs()
	extra, err := client.NewRedirectURI("http://localhost:3000/injected")
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}
	got = append(got, extra)

	if len(c.RedirectURIs()) != 1 {
		t.Fatalf("RedirectURIs() after external append = %d entries, want 1 (internal state must not be affected)", len(c.RedirectURIs()))
	}
	if len(got) != 2 {
		t.Fatalf("local slice after append = %d entries, want 2", len(got))
	}
}
