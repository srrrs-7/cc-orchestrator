package task_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// TestParsePriority covers R1 (the three known priority values are
// recognized and returned as their canonical Priority constant) and
// R5 (unknown and empty strings are rejected via the ErrInvalidPriority
// sentinel). ParsePriority is intentionally strict: unlike task
// creation (service.Create), it never defaults an empty string to
// medium — that default belongs to the application layer.
func TestParsePriority(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    task.Priority
		wantErr error
	}{
		{name: "low", input: "low", want: task.PriorityLow},
		{name: "medium", input: "medium", want: task.PriorityMedium},
		{name: "high", input: "high", want: task.PriorityHigh},
		{name: "unknown value is rejected (R5)", input: "urgent", wantErr: task.ErrInvalidPriority},
		{name: "empty string is rejected (R5, strict boundary)", input: "", wantErr: task.ErrInvalidPriority},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := task.ParsePriority(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParsePriority(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePriority(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParsePriority(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestPriority_String covers R4 (the wire representation of each
// Priority constant is exactly the low/medium/high enum shared with
// DTOs and JSON responses).
func TestPriority_String(t *testing.T) {
	tests := []struct {
		name string
		p    task.Priority
		want string
	}{
		{name: "low", p: task.PriorityLow, want: "low"},
		{name: "medium", p: task.PriorityMedium, want: "medium"},
		{name: "high", p: task.PriorityHigh, want: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
