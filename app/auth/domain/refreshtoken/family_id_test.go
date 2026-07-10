package refreshtoken_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// TestNewFamilyID covers the rotation-chain identifier: non-empty and
// cryptographically distinct across calls, so two independently
// issued refresh tokens never share a family (and thus never get
// revoked together by RevokeFamily).
func TestNewFamilyID(t *testing.T) {
	a, err := refreshtoken.NewFamilyID()
	if err != nil {
		t.Fatalf("NewFamilyID() unexpected error: %v", err)
	}
	if a.String() == "" {
		t.Error("String() is empty, want non-empty opaque value")
	}

	b, err := refreshtoken.NewFamilyID()
	if err != nil {
		t.Fatalf("NewFamilyID() unexpected error: %v", err)
	}
	if a.String() == b.String() {
		t.Error("two NewFamilyID() calls produced the same value, want cryptographically distinct random values")
	}
}

func TestParseFamilyID(t *testing.T) {
	t.Run("non-empty string succeeds and round-trips via String()", func(t *testing.T) {
		f, err := refreshtoken.ParseFamilyID("some-opaque-family-id")
		if err != nil {
			t.Fatalf("ParseFamilyID() unexpected error: %v", err)
		}
		if f.String() != "some-opaque-family-id" {
			t.Errorf("String() = %q, want %q", f.String(), "some-opaque-family-id")
		}
	})

	t.Run("empty string is rejected", func(t *testing.T) {
		_, err := refreshtoken.ParseFamilyID("")
		if !errors.Is(err, refreshtoken.ErrInvalidToken) {
			t.Fatalf("ParseFamilyID(\"\") error = %v, want wrapping %v", err, refreshtoken.ErrInvalidToken)
		}
	})
}
