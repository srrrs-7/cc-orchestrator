package authcode_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
)

func TestNewCode(t *testing.T) {
	a, err := authcode.NewCode()
	if err != nil {
		t.Fatalf("NewCode() unexpected error: %v", err)
	}
	if a.String() == "" {
		t.Error("String() is empty, want non-empty opaque value")
	}

	b, err := authcode.NewCode()
	if err != nil {
		t.Fatalf("NewCode() unexpected error: %v", err)
	}
	if a == b {
		t.Error("two NewCode() calls produced the same value, want cryptographically distinct random values")
	}
}

func TestParseCode(t *testing.T) {
	t.Run("non-empty string succeeds", func(t *testing.T) {
		c, err := authcode.ParseCode("some-opaque-code")
		if err != nil {
			t.Fatalf("ParseCode() unexpected error: %v", err)
		}
		if c.String() != "some-opaque-code" {
			t.Errorf("String() = %q, want %q", c.String(), "some-opaque-code")
		}
	})

	t.Run("empty string is rejected", func(t *testing.T) {
		_, err := authcode.ParseCode("")
		if !errors.Is(err, authcode.ErrNotFound) {
			t.Fatalf("ParseCode(\"\") error = %v, want wrapping %v", err, authcode.ErrNotFound)
		}
	})
}
