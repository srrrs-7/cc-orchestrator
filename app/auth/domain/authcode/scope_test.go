package authcode_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

// TestParseScope covers traceability #1 (openid REQUIRED in the
// authorization request's scope).
func TestParseScope(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    error
		wantHasOID bool
	}{
		{name: "openid alone succeeds", input: "openid", wantHasOID: true},
		{name: "openid plus extra scopes succeeds", input: "openid profile email", wantHasOID: true},
		{name: "openid missing is rejected", input: "profile email", wantErr: authcode.ErrMissingOpenIDScope},
		{name: "empty string is rejected", input: "", wantErr: authcode.ErrInvalidScope},
		{name: "whitespace-only string is rejected", input: "   ", wantErr: authcode.ErrInvalidScope},
		{name: "duplicated openid is accepted (dedup by set)", input: "openid openid", wantHasOID: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := authcode.ParseScope(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseScope(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseScope(%q) unexpected error: %v", tt.input, err)
			}
			if got.Has(authcode.ScopeOpenID) != tt.wantHasOID {
				t.Errorf("Has(openid) = %v, want %v", got.Has(authcode.ScopeOpenID), tt.wantHasOID)
			}
		})
	}
}

func TestScope_Has(t *testing.T) {
	scope, err := authcode.ParseScope("openid profile")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}

	if !scope.Has("profile") {
		t.Error("Has(\"profile\") = false, want true")
	}
	if scope.Has("email") {
		t.Error("Has(\"email\") = true, want false")
	}
}
