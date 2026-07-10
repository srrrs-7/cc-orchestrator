package migration

import (
	"strings"
	"testing"
)

// TestParseTarget_DefaultMigrationsDir covers the -target ->
// migrations-directory mapping (moved from the pre-refactor main.go's
// parseFlags/TestParseFlags_TargetDirAndDatabaseMapping): a recognized
// target's DefaultMigrationsDir is "/migrations/<target>", matching
// the Dockerfile's COPY layout.
func TestParseTarget_DefaultMigrationsDir(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		wantDir string
	}{
		{name: "api", target: "api", wantDir: "/migrations/api"},
		{name: "auth", target: "auth", wantDir: "/migrations/auth"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := ParseTarget(tc.target)
			if err != nil {
				t.Fatalf("ParseTarget(%q) unexpected error: %v", tc.target, err)
			}
			if got := target.DefaultMigrationsDir(); got != tc.wantDir {
				t.Errorf("ParseTarget(%q).DefaultMigrationsDir() = %q, want %q", tc.target, got, tc.wantDir)
			}
			if got := target.String(); got != tc.target {
				t.Errorf("ParseTarget(%q).String() = %q, want %q", tc.target, got, tc.target)
			}
		})
	}
}

// TestParseTarget_Unknown mirrors the pre-refactor
// TestParseFlags_UnknownTargetErrorNamesValue: an unrecognized target
// is rejected, and the error echoes the offending value so it is
// actionable.
func TestParseTarget_Unknown(t *testing.T) {
	_, err := ParseTarget("staging")
	if err == nil {
		t.Fatal("ParseTarget(\"staging\") = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("ParseTarget error = %q, want it to mention the invalid value %q", err.Error(), "staging")
	}
}

// TestParseTarget_Empty covers the "flag omitted entirely" case: an
// empty target is rejected, not silently defaulted.
func TestParseTarget_Empty(t *testing.T) {
	if _, err := ParseTarget(""); err == nil {
		t.Fatal("ParseTarget(\"\") = nil error, want an error")
	}
}
