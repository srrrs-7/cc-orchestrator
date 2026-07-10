package task_test

import (
	"errors"
	"regexp"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// idPattern matches the UUIDv4-shaped string produced by task.NewID:
// 8-4-4-4-12 lowercase hex digits, with the version (4) and variant
// ([89ab]) bits fixed as NewID sets them.
var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewID_Format(t *testing.T) {
	id := task.NewID()

	if !idPattern.MatchString(id.String()) {
		t.Errorf("NewID().String() = %q, want to match pattern %q", id.String(), idPattern.String())
	}
}

func TestNewID_Unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id := task.NewID().String()
		if seen[id] {
			t.Fatalf("NewID() produced duplicate value %q after %d iterations", id, i)
		}
		seen[id] = true
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "valid id is accepted as-is", input: "some-id"},
		{name: "empty string is rejected", input: "", wantErr: task.ErrInvalidID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := task.ParseID(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseID(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				// ISSUE-018: ParseID must produce a *task.ValidationError
				// (HTTP 400 category) so route can branch on the category
				// type instead of enumerating ErrInvalidID individually.
				var ve *task.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("ParseID(%q) error = %v, want errors.As(&task.ValidationError{}) = true", tt.input, err)
				}
				if ve.Msg != "invalid task id" {
					t.Errorf("ParseID(%q) ValidationError.Msg = %q, want %q", tt.input, ve.Msg, "invalid task id")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseID(%q) unexpected error: %v", tt.input, err)
			}
			if got.String() != tt.input {
				t.Errorf("ParseID(%q).String() = %q, want %q", tt.input, got.String(), tt.input)
			}
		})
	}
}
