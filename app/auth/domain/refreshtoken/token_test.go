package refreshtoken_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// TestNewToken covers the opaque plaintext Token: non-empty and
// cryptographically distinct across calls (crypto/rand, per SPEC-006
// non-functional requirements).
func TestNewToken(t *testing.T) {
	a, err := refreshtoken.NewToken()
	if err != nil {
		t.Fatalf("NewToken() unexpected error: %v", err)
	}
	if a.String() == "" {
		t.Error("String() is empty, want non-empty opaque value")
	}

	b, err := refreshtoken.NewToken()
	if err != nil {
		t.Fatalf("NewToken() unexpected error: %v", err)
	}
	if a.String() == b.String() {
		t.Error("two NewToken() calls produced the same value, want cryptographically distinct random values")
	}
}

// TestToken_Hash_MatchesHashToken covers R8: the hash stored for a
// Token (via Token.Hash()) must be identical to hashing the same
// plaintext independently through HashToken -- the equality the
// persistence layer relies on to look a presented refresh_token up by
// its hash.
func TestToken_Hash_MatchesHashToken(t *testing.T) {
	tok, err := refreshtoken.NewToken()
	if err != nil {
		t.Fatalf("setup NewToken() unexpected error: %v", err)
	}

	if tok.Hash() != refreshtoken.HashToken(tok.String()) {
		t.Errorf("Hash() = %v, want %v (HashToken(String()))", tok.Hash(), refreshtoken.HashToken(tok.String()))
	}
}

// TestHashToken_Deterministic covers R8 (SHA-256 hash used as the
// lookup key): identical plaintext must always hash to the same
// value, and distinct plaintext must (with overwhelming probability)
// hash to distinct values.
func TestHashToken_Deterministic(t *testing.T) {
	h1 := refreshtoken.HashToken("same-plaintext")
	h2 := refreshtoken.HashToken("same-plaintext")
	if h1 != h2 {
		t.Errorf("HashToken() of the same plaintext produced different hashes: %v != %v, want identical (deterministic)", h1, h2)
	}

	h3 := refreshtoken.HashToken("different-plaintext")
	if h1 == h3 {
		t.Error("HashToken() of two different plaintexts produced the same hash, want distinct hashes")
	}
}

func TestParseTokenHash(t *testing.T) {
	t.Run("non-empty string succeeds and round-trips via String()", func(t *testing.T) {
		h, err := refreshtoken.ParseTokenHash("0123456789abcdef")
		if err != nil {
			t.Fatalf("ParseTokenHash() unexpected error: %v", err)
		}
		if h.String() != "0123456789abcdef" {
			t.Errorf("String() = %q, want %q", h.String(), "0123456789abcdef")
		}
	})

	t.Run("empty string is rejected", func(t *testing.T) {
		_, err := refreshtoken.ParseTokenHash("")
		if !errors.Is(err, refreshtoken.ErrInvalidToken) {
			t.Fatalf("ParseTokenHash(\"\") error = %v, want wrapping %v", err, refreshtoken.ErrInvalidToken)
		}
	})
}
