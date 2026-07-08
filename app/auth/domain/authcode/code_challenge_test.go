package authcode_test

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

// s256Challenge independently computes the RFC 7636 S256 transformation
// (BASE64URL(SHA256(ASCII(verifier)))) using the standard library
// directly, without going through the authcode package's own
// implementation, so tests cross-check the expected value rather than
// merely re-running the same code under test.
func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestParseCodeChallengeMethod(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    authcode.CodeChallengeMethod
		wantErr error
	}{
		{name: "S256 is accepted", input: "S256", want: authcode.CodeChallengeMethodS256},
		{name: "plain is accepted as a value (type completeness)", input: "plain", want: authcode.CodeChallengeMethodPlain},
		{name: "unknown method is rejected", input: "foo", wantErr: authcode.ErrUnsupportedChallengeMethod},
		{name: "empty method is rejected", input: "", wantErr: authcode.ErrUnsupportedChallengeMethod},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := authcode.ParseCodeChallengeMethod(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseCodeChallengeMethod(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCodeChallengeMethod(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseCodeChallengeMethod(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewCodeChallenge(t *testing.T) {
	t.Run("valid challenge with S256 succeeds", func(t *testing.T) {
		cc, err := authcode.NewCodeChallenge("some-challenge-value", authcode.CodeChallengeMethodS256)
		if err != nil {
			t.Fatalf("NewCodeChallenge() unexpected error: %v", err)
		}
		if cc.Method() != authcode.CodeChallengeMethodS256 {
			t.Errorf("Method() = %v, want %v", cc.Method(), authcode.CodeChallengeMethodS256)
		}
	})

	t.Run("empty challenge is rejected", func(t *testing.T) {
		_, err := authcode.NewCodeChallenge("", authcode.CodeChallengeMethodS256)
		if !errors.Is(err, authcode.ErrInvalidCodeVerifier) {
			t.Fatalf("NewCodeChallenge(\"\", S256) error = %v, want wrapping %v", err, authcode.ErrInvalidCodeVerifier)
		}
	})

	t.Run("plain method is rejected (S256-only policy)", func(t *testing.T) {
		_, err := authcode.NewCodeChallenge("some-challenge-value", authcode.CodeChallengeMethodPlain)
		if !errors.Is(err, authcode.ErrUnsupportedChallengeMethod) {
			t.Fatalf("NewCodeChallenge(..., plain) error = %v, want wrapping %v", err, authcode.ErrUnsupportedChallengeMethod)
		}
	})
}

// TestCodeChallenge_Verify covers PKCE §3 of the plan's traceability
// table: S256 match (正), mismatch/invalid verifier (異), and the 43 /
// 128 character length boundary (境界).
func TestCodeChallenge_Verify(t *testing.T) {
	verifier43 := strings.Repeat("A", 43)
	verifier128 := strings.Repeat("A", 128)
	verifier42 := strings.Repeat("A", 42)
	verifier129 := strings.Repeat("A", 129)
	verifierInvalidChar := strings.Repeat("A", 42) + "!" // 43 chars, one invalid rune
	otherVerifier43 := strings.Repeat("B", 43)

	tests := []struct {
		name         string
		challenge    string
		codeVerifier string
		wantErr      error
	}{
		{
			name:         "S256 match succeeds",
			challenge:    s256Challenge(verifier43),
			codeVerifier: verifier43,
			wantErr:      nil,
		},
		{
			name:         "S256 mismatch fails",
			challenge:    s256Challenge(verifier43),
			codeVerifier: otherVerifier43,
			wantErr:      authcode.ErrPKCEVerificationFailed,
		},
		{
			name:         "verifier exactly 43 chars (lower boundary) succeeds",
			challenge:    s256Challenge(verifier43),
			codeVerifier: verifier43,
			wantErr:      nil,
		},
		{
			name:         "verifier exactly 128 chars (upper boundary) succeeds",
			challenge:    s256Challenge(verifier128),
			codeVerifier: verifier128,
			wantErr:      nil,
		},
		{
			name:         "verifier of 42 chars (just below lower boundary) is invalid",
			challenge:    s256Challenge(verifier42),
			codeVerifier: verifier42,
			wantErr:      authcode.ErrInvalidCodeVerifier,
		},
		{
			name:         "verifier of 129 chars (just above upper boundary) is invalid",
			challenge:    s256Challenge(verifier129),
			codeVerifier: verifier129,
			wantErr:      authcode.ErrInvalidCodeVerifier,
		},
		{
			name:         "verifier with a non-unreserved character is invalid",
			challenge:    s256Challenge(verifierInvalidChar),
			codeVerifier: verifierInvalidChar,
			wantErr:      authcode.ErrInvalidCodeVerifier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc, err := authcode.NewCodeChallenge(tt.challenge, authcode.CodeChallengeMethodS256)
			if err != nil {
				t.Fatalf("setup NewCodeChallenge() unexpected error: %v", err)
			}

			err = cc.Verify(tt.codeVerifier)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Verify(%q) error = %v, want wrapping %v", tt.codeVerifier, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify(%q) unexpected error: %v", tt.codeVerifier, err)
			}
		})
	}
}
