package repotest

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// NewUserRepository seeds exactly the given users into a fresh
// (empty) store and returns it as a user.Repository.
//
// user.Repository is read-only (FindByID / FindByUsername; see
// domain/user/repository.go): seeding is necessarily performed
// outside the interface, through postgres.SeedUser's idempotent
// upsert (wrapped by testsupport.SeedUser for tests; infra/postgres
// is the sole implementation since SPEC-011). Implementations MUST
// start from an empty store on every call so subtests never observe
// data left behind by another subtest.
type NewUserRepository func(t *testing.T, seed ...*user.User) user.Repository

// RunUserRepositoryContract runs the behavioral contract shared by
// every user.Repository implementation (SPEC-005 R2): FindByID and
// FindByUsername both round-trip a seeded User, both report
// user.ErrNotFound for an id/username that was never seeded, and
// FindByUsername performs an exact (case-sensitive) match.
func RunUserRepositoryContract(t *testing.T, newRepo NewUserRepository) {
	t.Helper()

	t.Run("FindByID finds a seeded user with every field round-tripped", func(t *testing.T) {
		u := newTestUser(t, "user-1", "demo-user", "s3cret", "Demo User", "demo@example.com")
		repo := newRepo(t, u)

		got, err := repo.FindByID(context.Background(), u.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		assertSameUser(t, got, u)
	})

	t.Run("FindByUsername finds a seeded user with every field round-tripped", func(t *testing.T) {
		u := newTestUser(t, "user-1", "demo-user", "s3cret", "Demo User", "demo@example.com")
		repo := newRepo(t, u)

		got, err := repo.FindByUsername(context.Background(), u.Username())
		if err != nil {
			t.Fatalf("FindByUsername() unexpected error: %v", err)
		}
		assertSameUser(t, got, u)
	})

	t.Run("FindByID for an id that was never seeded returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t) // no seed data at all
		unknownID, err := user.ParseUserID("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseUserID() unexpected error: %v", err)
		}

		_, err = repo.FindByID(context.Background(), unknownID)
		if !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("FindByID() error = %v, want wrapping %v", err, user.ErrNotFound)
		}
	})

	t.Run("FindByUsername for a username that was never seeded returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t) // no seed data at all
		unknownUsername, err := user.NewUsername("does-not-exist")
		if err != nil {
			t.Fatalf("setup NewUsername() unexpected error: %v", err)
		}

		_, err = repo.FindByUsername(context.Background(), unknownUsername)
		if !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("FindByUsername() error = %v, want wrapping %v", err, user.ErrNotFound)
		}
	})

	t.Run("FindByUsername performs an exact, case-sensitive match", func(t *testing.T) {
		u := newTestUser(t, "user-1", "Demo-User", "s3cret", "Demo User", "demo@example.com")
		repo := newRepo(t, u)

		differentCase, err := user.NewUsername("demo-user")
		if err != nil {
			t.Fatalf("setup NewUsername() unexpected error: %v", err)
		}
		if differentCase == u.Username() {
			t.Fatalf("test setup invalid: %q and %q must differ only by case", differentCase, u.Username())
		}

		if _, err := repo.FindByUsername(context.Background(), differentCase); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("FindByUsername(differently-cased) error = %v, want wrapping %v (lookup must be case-sensitive)", err, user.ErrNotFound)
		}
		if _, err := repo.FindByUsername(context.Background(), u.Username()); err != nil {
			t.Errorf("FindByUsername(exact case) unexpected error: %v", err)
		}
	})

	t.Run("ListAll returns every seeded user ordered by id", func(t *testing.T) {
		u1 := newTestUser(t, "user-a", "alice", "s3cret", "Alice", "alice@example.com")
		u2 := newTestUser(t, "user-b", "bob", "s3cret", "Bob", "bob@example.com")
		repo := newRepo(t, u2, u1)

		got, err := repo.ListAll(context.Background())
		if err != nil {
			t.Fatalf("ListAll() unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListAll() len = %d, want 2", len(got))
		}
		if got[0].ID().String() != "user-a" || got[1].ID().String() != "user-b" {
			t.Errorf("ListAll() order = [%q, %q], want [user-a, user-b]", got[0].ID(), got[1].ID())
		}
	})

	t.Run("ListAll on an empty store returns an empty slice", func(t *testing.T) {
		repo := newRepo(t)

		got, err := repo.ListAll(context.Background())
		if err != nil {
			t.Fatalf("ListAll() unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("ListAll() len = %d, want 0", len(got))
		}
	})
}

func newTestUser(t *testing.T, id, username, password, profileName, profileEmail string) *user.User {
	t.Helper()

	userID, err := user.ParseUserID(id)
	if err != nil {
		t.Fatalf("setup ParseUserID(%q) unexpected error: %v", id, err)
	}
	uname, err := user.NewUsername(username)
	if err != nil {
		t.Fatalf("setup NewUsername(%q) unexpected error: %v", username, err)
	}
	profile, err := user.NewProfile(profileName, profileEmail)
	if err != nil {
		t.Fatalf("setup NewProfile(%q, %q) unexpected error: %v", profileName, profileEmail, err)
	}
	u, err := user.New(userID, uname, password, profile)
	if err != nil {
		t.Fatalf("setup user.New() unexpected error: %v", err)
	}
	return u
}

func assertSameUser(t *testing.T, got, want *user.User) {
	t.Helper()
	if got.ID() != want.ID() {
		t.Errorf("ID() = %v, want %v", got.ID(), want.ID())
	}
	if got.Username() != want.Username() {
		t.Errorf("Username() = %v, want %v", got.Username(), want.Username())
	}
	if got.PasswordHash() != want.PasswordHash() {
		t.Errorf("PasswordHash() = %v, want %v", got.PasswordHash(), want.PasswordHash())
	}
	if got.Profile().Name() != want.Profile().Name() {
		t.Errorf("Profile().Name() = %v, want %v", got.Profile().Name(), want.Profile().Name())
	}
	if got.Profile().Email() != want.Profile().Email() {
		t.Errorf("Profile().Email() = %v, want %v", got.Profile().Email(), want.Profile().Email())
	}
}
