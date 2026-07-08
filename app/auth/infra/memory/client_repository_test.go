package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
)

func newSeededClient(t *testing.T) *client.Client {
	t.Helper()

	id, err := client.ParseClientID("demo-client")
	if err != nil {
		t.Fatalf("setup ParseClientID() unexpected error: %v", err)
	}
	redirectURI, err := client.NewRedirectURI("http://localhost:3000/callback")
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}
	return client.New(id, []client.RedirectURI{redirectURI}, []string{"openid"}, []string{"code"}, []string{"authorization_code"})
}

func TestClientRepository_FindByID(t *testing.T) {
	t.Run("seeded client is found", func(t *testing.T) {
		repo := memory.NewClientRepository()
		c := newSeededClient(t)
		repo.Seed(c)

		got, err := repo.FindByID(context.Background(), c.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		if got.ID() != c.ID() {
			t.Errorf("ID() = %v, want %v", got.ID(), c.ID())
		}
	})

	t.Run("unseeded id is not found", func(t *testing.T) {
		repo := memory.NewClientRepository()

		unknownID, err := client.ParseClientID("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseClientID() unexpected error: %v", err)
		}

		_, err = repo.FindByID(context.Background(), unknownID)
		if !errors.Is(err, client.ErrNotFound) {
			t.Fatalf("FindByID() error = %v, want wrapping %v", err, client.ErrNotFound)
		}
	})
}

// TestClientRepository_CloneIndependence verifies the repository
// returns an independent clone on every read: successive FindByID
// calls must not return the very same *client.Client instance, so
// that a caller could never accidentally mutate the repository's
// internal state through a returned pointer.
func TestClientRepository_CloneIndependence(t *testing.T) {
	repo := memory.NewClientRepository()
	c := newSeededClient(t)
	repo.Seed(c)

	got1, err := repo.FindByID(context.Background(), c.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	got2, err := repo.FindByID(context.Background(), c.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}

	if got1 == got2 {
		t.Error("two FindByID() calls returned the same pointer, want independent clones")
	}
	if got1 == c {
		t.Error("FindByID() returned the exact pointer passed to Seed(), want an independent clone")
	}
}
