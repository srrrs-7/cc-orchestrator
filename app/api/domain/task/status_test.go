package task_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

func TestStatus_CanTransitionTo(t *testing.T) {
	// Full transition table for the three known statuses: only
	// todo->doing and doing->done are allowed, everything else
	// (including self-transitions and backward transitions) is not.
	statuses := []task.Status{task.StatusTodo, task.StatusDoing, task.StatusDone}

	tests := []struct {
		from task.Status
		to   task.Status
		want bool
	}{
		{task.StatusTodo, task.StatusTodo, false},
		{task.StatusTodo, task.StatusDoing, true},
		{task.StatusTodo, task.StatusDone, false},
		{task.StatusDoing, task.StatusTodo, false},
		{task.StatusDoing, task.StatusDoing, false},
		{task.StatusDoing, task.StatusDone, true},
		{task.StatusDone, task.StatusTodo, false},
		{task.StatusDone, task.StatusDoing, false},
		{task.StatusDone, task.StatusDone, false},
	}

	if len(tests) != len(statuses)*len(statuses) {
		t.Fatalf("transition table incomplete: have %d cases, want %d (all status pairs)", len(tests), len(statuses)*len(statuses))
	}

	for _, tt := range tests {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.want {
				t.Errorf("CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    task.Status
		wantErr error
	}{
		{name: "todo", input: "todo", want: task.StatusTodo},
		{name: "doing", input: "doing", want: task.StatusDoing},
		{name: "done", input: "done", want: task.StatusDone},
		{name: "unknown value is rejected", input: "unknown", wantErr: task.ErrInvalidStatus},
		{name: "empty string is rejected", input: "", wantErr: task.ErrInvalidStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := task.ParseStatus(tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseStatus(%q) error = %v, want wrapping %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseStatus(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
