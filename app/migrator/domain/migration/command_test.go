package migration

import (
	"strings"
	"testing"
)

// TestParseCommand covers the closed -command value set (SPEC-005 R5)
// : the three recognized goose commands round-trip through String,
// and the default ("up", applied by cmd/migrator's flag parsing, not
// here) is among them.
func TestParseCommand(t *testing.T) {
	for _, want := range []string{"up", "down", "status"} {
		t.Run(want, func(t *testing.T) {
			cmd, err := ParseCommand(want)
			if err != nil {
				t.Fatalf("ParseCommand(%q) unexpected error: %v", want, err)
			}
			if got := cmd.String(); got != want {
				t.Errorf("ParseCommand(%q).String() = %q, want %q", want, got, want)
			}
		})
	}
}

// TestParseCommand_Unknown mirrors the pre-refactor
// TestParseFlags_UnknownCommandErrorNamesValue: an unrecognized
// command is rejected and the error echoes the offending value.
func TestParseCommand_Unknown(t *testing.T) {
	_, err := ParseCommand("reset")
	if err == nil {
		t.Fatal("ParseCommand(\"reset\") = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "reset") {
		t.Errorf("ParseCommand error = %q, want it to mention the invalid value %q", err.Error(), "reset")
	}
}
