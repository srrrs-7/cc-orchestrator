package user_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

func newTestUser(t *testing.T, password string) *user.User {
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

	u, err := user.New(id, username, password, profile)
	if err != nil {
		t.Fatalf("setup user.New() unexpected error: %v", err)
	}
	return u
}

func TestUser_VerifyPassword(t *testing.T) {
	tests := []struct {
		name      string
		stored    string
		candidate string
		want      bool
	}{
		{name: "matching password succeeds", stored: "s3cret", candidate: "s3cret", want: true},
		{name: "mismatching password fails", stored: "s3cret", candidate: "wrong", want: false},
		{name: "empty candidate fails even against an empty stored password (boundary)", stored: "", candidate: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newTestUser(t, tt.stored)

			if got := u.VerifyPassword(tt.candidate); got != tt.want {
				t.Errorf("VerifyPassword(%q) = %v, want %v", tt.candidate, got, tt.want)
			}
		})
	}
}

func TestUser_Getters(t *testing.T) {
	u := newTestUser(t, "s3cret")

	if u.ID().String() != "user-1" {
		t.Errorf("ID().String() = %q, want %q", u.ID().String(), "user-1")
	}
	if u.Username().String() != "demo-user" {
		t.Errorf("Username().String() = %q, want %q", u.Username().String(), "demo-user")
	}
	if u.Profile().Email() != "demo@example.com" {
		t.Errorf("Profile().Email() = %q, want %q", u.Profile().Email(), "demo@example.com")
	}
	if u.PasswordHash() == "" {
		t.Error("PasswordHash() must not be empty after New")
	}
}
