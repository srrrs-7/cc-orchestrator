package task_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

func TestNewTitle(t *testing.T) {
	exactly100 := strings.Repeat("あ", 100) // multi-byte rune, exactly at the boundary
	over100 := strings.Repeat("あ", 101)    // one rune past the boundary

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
		wantMsg string
	}{
		{name: "normal title", input: "buy milk", want: "buy milk"},
		{name: "trims surrounding whitespace", input: "  buy milk  ", want: "buy milk"},
		{name: "empty string is rejected", input: "", wantErr: task.ErrEmptyTitle, wantMsg: "title must not be empty"},
		{name: "whitespace only is rejected", input: "   \t\n ", wantErr: task.ErrEmptyTitle, wantMsg: "title must not be empty"},
		{name: "exactly 100 runes is ok (boundary)", input: exactly100, want: exactly100},
		{name: "101 runes is too long (boundary)", input: over100, wantErr: task.ErrTitleTooLong, wantMsg: "title is too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := task.NewTitle(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewTitle(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				// ISSUE-018: NewTitle must produce a *task.ValidationError
				// (HTTP 400 category), not just a bare sentinel-wrapping
				// error, so route can branch on the category type.
				var ve *task.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("NewTitle(%q) error = %v, want errors.As(&task.ValidationError{}) = true", tt.input, err)
				}
				if ve.Msg != tt.wantMsg {
					t.Errorf("NewTitle(%q) ValidationError.Msg = %q, want %q", tt.input, ve.Msg, tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewTitle(%q) unexpected error: %v", tt.input, err)
			}
			if got.String() != tt.want {
				t.Errorf("NewTitle(%q).String() = %q, want %q", tt.input, got.String(), tt.want)
			}
		})
	}
}
