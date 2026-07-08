package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
)

func newSeededUser(t *testing.T) *user.User {
	t.Helper()

	id, err := user.ParseUserID("user-1")
	if err != nil {
		t.Fatalf("setup ParseUserID() unexpected error: %v", err)
	}
	username, err := user.NewUsername("demo-user")
	if err != nil {
		t.Fatalf("setup NewUsername() unexpected error: %v", err)
	}
	profile, err := user.NewProfile("Demo User", "demo@example.com")
	if err != nil {
		t.Fatalf("setup NewProfile() unexpected error: %v", err)
	}
	return user.New(id, username, "s3cret", profile)
}

func TestUserRepository_FindByID(t *testing.T) {
	t.Run("seeded user is found", func(t *testing.T) {
		repo := memory.NewUserRepository()
		u := newSeededUser(t)
		repo.Seed(u)

		got, err := repo.FindByID(context.Background(), u.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		if got.ID() != u.ID() {
			t.Errorf("ID() = %v, want %v", got.ID(), u.ID())
		}
	})

	t.Run("unseeded id is not found", func(t *testing.T) {
		repo := memory.NewUserRepository()

		unknownID, err := user.ParseUserID("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseUserID() unexpected error: %v", err)
		}

		_, err = repo.FindByID(context.Background(), unknownID)
		if !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("FindByID() error = %v, want wrapping %v", err, user.ErrNotFound)
		}
	})
}

func TestUserRepository_FindByUsername(t *testing.T) {
	t.Run("seeded username is found", func(t *testing.T) {
		repo := memory.NewUserRepository()
		u := newSeededUser(t)
		repo.Seed(u)

		got, err := repo.FindByUsername(context.Background(), u.Username())
		if err != nil {
			t.Fatalf("FindByUsername() unexpected error: %v", err)
		}
		if got.ID() != u.ID() {
			t.Errorf("ID() = %v, want %v", got.ID(), u.ID())
		}
	})

	t.Run("unseeded username is not found", func(t *testing.T) {
		repo := memory.NewUserRepository()

		unknownUsername, err := user.NewUsername("does-not-exist")
		if err != nil {
			t.Fatalf("setup NewUsername() unexpected error: %v", err)
		}

		_, err = repo.FindByUsername(context.Background(), unknownUsername)
		if !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("FindByUsername() error = %v, want wrapping %v", err, user.ErrNotFound)
		}
	})
}

// TestUserRepository_CloneIndependence verifies the repository
// returns an independent clone on every read.
func TestUserRepository_CloneIndependence(t *testing.T) {
	repo := memory.NewUserRepository()
	u := newSeededUser(t)
	repo.Seed(u)

	got1, err := repo.FindByID(context.Background(), u.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}
	got2, err := repo.FindByID(context.Background(), u.ID())
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}

	if got1 == got2 {
		t.Error("two FindByID() calls returned the same pointer, want independent clones")
	}
	if got1 == u {
		t.Error("FindByID() returned the exact pointer passed to Seed(), want an independent clone")
	}
}
