package authcode_test

import (
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

func TestNonce(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEmpty bool
	}{
		{name: "empty string is a valid, empty nonce (no nonce requested)", input: "", wantEmpty: true},
		{name: "non-empty value is not empty", input: "abc123", wantEmpty: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := authcode.NewNonce(tt.input)

			if n.String() != tt.input {
				t.Errorf("String() = %q, want %q", n.String(), tt.input)
			}
			if n.IsEmpty() != tt.wantEmpty {
				t.Errorf("IsEmpty() = %v, want %v", n.IsEmpty(), tt.wantEmpty)
			}
		})
	}
}
