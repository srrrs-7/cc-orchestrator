package refreshtoken_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// TestParseScope covers that refreshtoken.Scope, unlike
// domain/authcode.Scope, does NOT require "openid" to be present
// (SPEC-006 plan §リスク "refreshtoken.Scope は openid 必須にしない = 他
// domain 非結合"): only emptiness is rejected.
func TestParseScope(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "single scope value succeeds", input: "openid"},
		{name: "multiple scope values succeed", input: "openid profile email"},
		{name: "openid absent still succeeds (refreshtoken.Scope has no openid requirement)", input: "profile email"},
		{name: "empty string is rejected", input: "", wantErr: refreshtoken.ErrInvalidScope},
		{name: "whitespace-only string is rejected", input: "   ", wantErr: refreshtoken.ErrInvalidScope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := refreshtoken.ParseScope(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseScope(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseScope(%q) unexpected error: %v", tt.input, err)
			}
			if got.String() == "" {
				t.Errorf("ParseScope(%q).String() is empty, want non-empty", tt.input)
			}
		})
	}
}

func TestScope_Has(t *testing.T) {
	scope, err := refreshtoken.ParseScope("openid profile")
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

func TestScope_Values(t *testing.T) {
	scope, err := refreshtoken.ParseScope("openid profile")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}

	values := scope.Values()
	if len(values) != 2 {
		t.Fatalf("Values() = %v, want 2 entries", values)
	}
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		seen[v] = true
	}
	if !seen["openid"] || !seen["profile"] {
		t.Errorf("Values() = %v, want to contain openid and profile", values)
	}
}

// TestScope_Narrow covers R7: a requested scope must be a subset of
// the original grant (subset allowed, equal allowed, widening
// rejected as ErrInvalidScope, disjoint rejected), and an empty
// request keeps the original scope unchanged (mirrors the
// service-layer shortcut documented in
// docs/plans/SPEC-006-plan.md "service リフレッシュフロー" step 6, but is
// exercised here directly against the domain method since the service
// is free to call Narow("") instead of special-casing empty itself).
func TestScope_Narrow(t *testing.T) {
	original, err := refreshtoken.ParseScope("openid profile email")
	if err != nil {
		t.Fatalf("setup ParseScope() unexpected error: %v", err)
	}

	t.Run("subset request succeeds and drops the omitted value", func(t *testing.T) {
		narrowed, err := original.Narrow("openid profile")
		if err != nil {
			t.Fatalf("Narrow() unexpected error: %v", err)
		}
		if !narrowed.Has("openid") || !narrowed.Has("profile") {
			t.Errorf("Narrow(\"openid profile\") = %v, want to contain openid and profile", narrowed)
		}
		if narrowed.Has("email") {
			t.Errorf("Narrow(\"openid profile\") = %v, want it to not contain email (not requested)", narrowed)
		}
	})

	t.Run("equal request succeeds and keeps every value", func(t *testing.T) {
		narrowed, err := original.Narrow("openid profile email")
		if err != nil {
			t.Fatalf("Narrow() unexpected error: %v", err)
		}
		if len(narrowed.Values()) != len(original.Values()) {
			t.Errorf("Narrow() with the exact original scope = %v, want the same %d values as the original", narrowed, len(original.Values()))
		}
	})

	t.Run("empty request keeps the original scope unchanged", func(t *testing.T) {
		narrowed, err := original.Narrow("")
		if err != nil {
			t.Fatalf("Narrow(\"\") unexpected error: %v", err)
		}
		if len(narrowed.Values()) != len(original.Values()) {
			t.Errorf("Narrow(\"\") = %v, want the original scope unchanged (%v)", narrowed, original)
		}
		for _, v := range original.Values() {
			if !narrowed.Has(v) {
				t.Errorf("Narrow(\"\") result missing original scope value %q", v)
			}
		}
	})

	t.Run("widening request is rejected", func(t *testing.T) {
		_, err := original.Narrow("openid profile email admin")
		if !errors.Is(err, refreshtoken.ErrInvalidScope) {
			t.Fatalf("Narrow(\"openid profile email admin\") error = %v, want wrapping %v", err, refreshtoken.ErrInvalidScope)
		}
	})

	t.Run("disjoint request is rejected", func(t *testing.T) {
		_, err := original.Narrow("admin")
		if !errors.Is(err, refreshtoken.ErrInvalidScope) {
			t.Fatalf("Narrow(\"admin\") error = %v, want wrapping %v", err, refreshtoken.ErrInvalidScope)
		}
	})
}
