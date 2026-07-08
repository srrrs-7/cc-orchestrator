package user_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

func TestParseUserID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "non-empty value succeeds", input: "user-1", want: "user-1"},
		{name: "empty value is rejected", input: "", wantErr: user.ErrInvalidUserID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := user.ParseUserID(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseUserID(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUserID(%q) unexpected error: %v", tt.input, err)
			}
			if got.String() != tt.want {
				t.Errorf("String() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}
