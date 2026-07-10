package user_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

func TestNewProfile(t *testing.T) {
	tests := []struct {
		name      string
		inputName string
		email     string
		wantErr   error
	}{
		{name: "well-formed email succeeds", inputName: "Demo User", email: "demo@example.com"},
		{name: "minimal single-char local/domain email succeeds (boundary)", inputName: "", email: "a@b"},
		{name: "empty email is rejected", inputName: "Demo User", email: "", wantErr: user.ErrInvalidEmail},
		{name: "email without @ is rejected", inputName: "Demo User", email: "demo-example.com", wantErr: user.ErrInvalidEmail},
		{name: "email with empty local part is rejected", inputName: "Demo User", email: "@example.com", wantErr: user.ErrInvalidEmail},
		{name: "email with empty domain part is rejected", inputName: "Demo User", email: "demo@", wantErr: user.ErrInvalidEmail},
		{name: "email with multiple @ is rejected", inputName: "Demo User", email: "demo@ex@ample.com", wantErr: user.ErrInvalidEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := user.NewProfile(tt.inputName, tt.email)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewProfile(%q, %q) error = %v, want wrapping %v", tt.inputName, tt.email, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewProfile(%q, %q) unexpected error: %v", tt.inputName, tt.email, err)
			}
			if got.Email() != tt.email {
				t.Errorf("Email() = %q, want %q", got.Email(), tt.email)
			}
		})
	}
}
