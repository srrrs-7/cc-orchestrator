package user_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

func TestNewUsername(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "non-empty value succeeds", input: "demo-user", want: "demo-user"},
		{name: "surrounding whitespace is trimmed (boundary)", input: "  demo-user  ", want: "demo-user"},
		{name: "empty value is rejected", input: "", wantErr: user.ErrInvalidUsername},
		{name: "whitespace-only value is rejected", input: "   ", wantErr: user.ErrInvalidUsername},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := user.NewUsername(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewUsername(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewUsername(%q) unexpected error: %v", tt.input, err)
			}
			if got.String() != tt.want {
				t.Errorf("String() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}
