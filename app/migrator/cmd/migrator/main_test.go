package main

import (
	"strings"
	"testing"
)

// TestParseFlags_CommandAndDirDefaults exercises parseFlags's CLI
// contract (SPEC-005 plan §RF.2.1 "-target"/"-command"/
// "-migrations-dir"): a recognized target selects "/migrations/<target>"
// by default (via migration.Target.DefaultMigrationsDir, tested
// directly in domain/migration/target_test.go), the default command is
// "up", and an explicit -migrations-dir overrides the target-derived
// default. Moved from the pre-refactor main_test.go's
// TestParseFlags_TargetDirAndDatabaseMapping; the target->dir mapping
// cases themselves moved to domain/migration/target_test.go
// (TestParseTarget_DefaultMigrationsDir), since that mapping is now a
// domain-level fact, not something parseFlags computes itself.
func TestParseFlags_CommandAndDirDefaults(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantTarget  string
		wantCommand string
		wantDir     string
		wantErr     bool
	}{
		{
			name:        "target=api, no -command, no -migrations-dir -> default dir /migrations/api, default command up",
			args:        []string{"-target", "api"},
			wantTarget:  "api",
			wantCommand: "up",
			wantDir:     "/migrations/api",
		},
		{
			name:        "target=auth, no -command -> default dir /migrations/auth",
			args:        []string{"-target", "auth"},
			wantTarget:  "auth",
			wantCommand: "up",
			wantDir:     "/migrations/auth",
		},
		{
			name:        "target=api, -command down -> dir still derives from target, not command",
			args:        []string{"-target", "api", "-command", "down"},
			wantTarget:  "api",
			wantCommand: "down",
			wantDir:     "/migrations/api",
		},
		{
			name:        "target=auth, -command status",
			args:        []string{"-target", "auth", "-command", "status"},
			wantTarget:  "auth",
			wantCommand: "status",
			wantDir:     "/migrations/auth",
		},
		{
			name:        "explicit -migrations-dir overrides the target-derived default",
			args:        []string{"-target", "api", "-migrations-dir", "/tmp/custom/dir"},
			wantTarget:  "api",
			wantCommand: "up",
			wantDir:     "/tmp/custom/dir",
		},
		{
			name:    "unknown target is rejected",
			args:    []string{"-target", "staging"},
			wantErr: true,
		},
		{
			name:    "empty target (flag omitted entirely) is rejected, not defaulted",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "unknown command is rejected even with a valid target",
			args:    []string{"-target", "api", "-command", "reset"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, command, dir, err := parseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFlags(%v) = (%q, %q, %q, nil), want an error", tc.args, target, command, dir)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v) unexpected error: %v", tc.args, err)
			}
			if target.String() != tc.wantTarget {
				t.Errorf("parseFlags(%v) target = %q, want %q", tc.args, target, tc.wantTarget)
			}
			if command.String() != tc.wantCommand {
				t.Errorf("parseFlags(%v) command = %q, want %q", tc.args, command, tc.wantCommand)
			}
			if dir != tc.wantDir {
				t.Errorf("parseFlags(%v) migrationsDir = %q, want %q", tc.args, dir, tc.wantDir)
			}
		})
	}
}

// TestParseFlags_UnknownTargetErrorNamesValue asserts the -target
// validation error is actionable: it echoes back the offending value
// (via the wrapped migration.ParseTarget error) and names the -target
// flag itself (via parseFlags's own wrap), so an operator (or an ECS
// task's exit-code-only log) can tell what was actually passed.
func TestParseFlags_UnknownTargetErrorNamesValue(t *testing.T) {
	_, _, _, err := parseFlags([]string{"-target", "staging"})
	if err == nil {
		t.Fatal("parseFlags(-target staging) = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("parseFlags error = %q, want it to mention the invalid value %q", err.Error(), "staging")
	}
	if !strings.Contains(err.Error(), "-target") {
		t.Errorf("parseFlags error = %q, want it to mention -target", err.Error())
	}
}

// TestParseFlags_UnknownCommandErrorNamesValue mirrors the above for
// -command.
func TestParseFlags_UnknownCommandErrorNamesValue(t *testing.T) {
	_, _, _, err := parseFlags([]string{"-target", "api", "-command", "reset"})
	if err == nil {
		t.Fatal("parseFlags(-command reset) = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "reset") {
		t.Errorf("parseFlags error = %q, want it to mention the invalid value %q", err.Error(), "reset")
	}
	if !strings.Contains(err.Error(), "-command") {
		t.Errorf("parseFlags error = %q, want it to mention -command", err.Error())
	}
}
