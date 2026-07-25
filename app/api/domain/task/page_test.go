package task_test

import (
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
)

// intPtr returns a pointer to i, for building *int limit/offset
// arguments to task.NewPage in table-driven tests below.
func intPtr(i int) *int {
	return &i
}

// TestNewPage_Defaults covers R1 (SPEC-008): a nil limit defaults to
// task.DefaultLimit (20) and a nil offset defaults to 0, whether both
// are unspecified or only one of them is.
func TestNewPage_Defaults(t *testing.T) {
	tests := []struct {
		name       string
		limit      *int
		offset     *int
		wantLimit  int
		wantOffset int
	}{
		{name: "both nil default to 20/0", limit: nil, offset: nil, wantLimit: task.DefaultLimit, wantOffset: 0},
		{name: "nil limit with explicit offset still defaults limit to 20", limit: nil, offset: intPtr(40), wantLimit: task.DefaultLimit, wantOffset: 40},
		{name: "explicit limit with nil offset still defaults offset to 0", limit: intPtr(5), offset: nil, wantLimit: 5, wantOffset: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := task.NewPage(tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("NewPage(%v, %v) unexpected error: %v", tt.limit, tt.offset, err)
			}
			if page.Limit() != tt.wantLimit {
				t.Errorf("Limit() = %d, want %d", page.Limit(), tt.wantLimit)
			}
			if page.Offset() != tt.wantOffset {
				t.Errorf("Offset() = %d, want %d", page.Offset(), tt.wantOffset)
			}
		})
	}
}

// TestNewPage_ClampsLimitAboveMax covers R3: a limit greater than
// task.MaxLimit (100) is silently clamped to MaxLimit rather than
// rejected, and the boundary value MaxLimit itself is passed through
// unchanged (not treated as "above max").
func TestNewPage_ClampsLimitAboveMax(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		wantLimit int
	}{
		{name: "just above max (101) clamps to 100", limit: task.MaxLimit + 1, wantLimit: task.MaxLimit},
		{name: "far above max (1000) clamps to 100", limit: 1000, wantLimit: task.MaxLimit},
		{name: "exactly at max (100) is not clamped, boundary", limit: task.MaxLimit, wantLimit: task.MaxLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := task.NewPage(intPtr(tt.limit), nil)
			if err != nil {
				t.Fatalf("NewPage(%d, nil) unexpected error: %v", tt.limit, err)
			}
			if page.Limit() != tt.wantLimit {
				t.Errorf("Limit() = %d, want %d", page.Limit(), tt.wantLimit)
			}
		})
	}
}

// TestNewPage_InvalidLimit covers R3's lower bound: a limit less than
// 1 (zero or negative) is rejected with a *task.ValidationError
// wrapping task.ErrInvalidLimit, not silently clamped/defaulted.
func TestNewPage_InvalidLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
	}{
		{name: "zero is rejected", limit: 0},
		{name: "negative is rejected", limit: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := task.NewPage(intPtr(tt.limit), nil)

			if !errors.Is(err, task.ErrInvalidLimit) {
				t.Fatalf("NewPage(%d, nil) error = %v, want wrapping %v", tt.limit, err, task.ErrInvalidLimit)
			}
			var ve *task.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("NewPage(%d, nil) error = %v, want errors.As(&task.ValidationError{}) = true", tt.limit, err)
			}
		})
	}
}

// TestNewPage_InvalidOffset covers R3: a negative offset is rejected
// with a *task.ValidationError wrapping task.ErrInvalidOffset, while
// the boundary value 0 is accepted.
func TestNewPage_InvalidOffset(t *testing.T) {
	t.Run("negative offset is rejected", func(t *testing.T) {
		_, err := task.NewPage(nil, intPtr(-1))

		if !errors.Is(err, task.ErrInvalidOffset) {
			t.Fatalf("NewPage(nil, -1) error = %v, want wrapping %v", err, task.ErrInvalidOffset)
		}
		var ve *task.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("NewPage(nil, -1) error = %v, want errors.As(&task.ValidationError{}) = true", err)
		}
	})

	t.Run("zero offset is accepted, boundary", func(t *testing.T) {
		page, err := task.NewPage(nil, intPtr(0))
		if err != nil {
			t.Fatalf("NewPage(nil, 0) unexpected error: %v", err)
		}
		if page.Offset() != 0 {
			t.Errorf("Offset() = %d, want 0", page.Offset())
		}
	})
}

// TestNewPage_MaxOffset covers ISSUE-025 item D: an offset above
// task.MaxOffset is rejected with a *task.ValidationError wrapping
// task.ErrInvalidOffset; the boundary value MaxOffset itself is
// accepted.
func TestNewPage_MaxOffset(t *testing.T) {
	tests := []struct {
		name    string
		offset  int
		wantErr bool
	}{
		{name: "just above max (MaxOffset+1) is rejected", offset: task.MaxOffset + 1, wantErr: true},
		{name: "far above max (999999999) is rejected", offset: 999999999, wantErr: true},
		{name: "exactly at max (MaxOffset) is accepted, boundary", offset: task.MaxOffset, wantErr: false},
		{name: "one below max (MaxOffset-1) is accepted", offset: task.MaxOffset - 1, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := task.NewPage(nil, intPtr(tt.offset))
			if tt.wantErr {
				if !errors.Is(err, task.ErrInvalidOffset) {
					t.Fatalf("NewPage(nil, %d) error = %v, want wrapping %v", tt.offset, err, task.ErrInvalidOffset)
				}
				var ve *task.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("NewPage(nil, %d) error = %v, want errors.As(&task.ValidationError{}) = true", tt.offset, err)
				}
			} else {
				if err != nil {
					t.Fatalf("NewPage(nil, %d) unexpected error: %v", tt.offset, err)
				}
				if page.Offset() != tt.offset {
					t.Errorf("Offset() = %d, want %d", page.Offset(), tt.offset)
				}
			}
		})
	}
}

// TestNewPage_BoundaryLimitOne covers the lower boundary that is
// valid (as opposed to TestNewPage_InvalidLimit's zero/negative
// rejection): limit=1 is the smallest accepted value and must be
// passed through unchanged.
func TestNewPage_BoundaryLimitOne(t *testing.T) {
	page, err := task.NewPage(intPtr(1), nil)
	if err != nil {
		t.Fatalf("NewPage(1, nil) unexpected error: %v", err)
	}
	if page.Limit() != 1 {
		t.Errorf("Limit() = %d, want 1", page.Limit())
	}
}

// TestNewPage_NormalValues covers the plain (non-boundary,
// non-defaulted) case: both limit and offset supplied within range
// are applied verbatim.
func TestNewPage_NormalValues(t *testing.T) {
	page, err := task.NewPage(intPtr(20), intPtr(40))
	if err != nil {
		t.Fatalf("NewPage(20, 40) unexpected error: %v", err)
	}
	if page.Limit() != 20 {
		t.Errorf("Limit() = %d, want 20", page.Limit())
	}
	if page.Offset() != 40 {
		t.Errorf("Offset() = %d, want 40", page.Offset())
	}
}
