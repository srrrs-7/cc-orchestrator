package client_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
)

func TestNewRedirectURI(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "absolute https URI succeeds", input: "https://example.com/callback"},
		{name: "absolute http URI succeeds", input: "http://localhost:3000/callback"},
		{name: "empty string is rejected", input: "", wantErr: client.ErrInvalidRedirectURI},
		{name: "relative path is rejected (not absolute)", input: "/callback", wantErr: client.ErrInvalidRedirectURI},
		{name: "non-http(s) scheme is rejected", input: "ftp://example.com/callback", wantErr: client.ErrInvalidRedirectURI},
		{name: "custom scheme without host is rejected", input: "myapp:/callback", wantErr: client.ErrInvalidRedirectURI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.NewRedirectURI(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewRedirectURI(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewRedirectURI(%q) unexpected error: %v", tt.input, err)
			}
			if got.String() != tt.input {
				t.Errorf("String() = %q, want %q", got.String(), tt.input)
			}
		})
	}
}

func TestRedirectURI_Equal(t *testing.T) {
	a, err := client.NewRedirectURI("https://example.com/callback")
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}
	same, err := client.NewRedirectURI("https://example.com/callback")
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}
	different, err := client.NewRedirectURI("https://example.com/other")
	if err != nil {
		t.Fatalf("setup NewRedirectURI() unexpected error: %v", err)
	}

	if !a.Equal(same) {
		t.Error("Equal() = false for identical redirect URIs, want true")
	}
	if a.Equal(different) {
		t.Error("Equal() = true for different redirect URIs, want false")
	}
}
