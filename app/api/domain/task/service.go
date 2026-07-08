package task

import (
	"context"
	"errors"
	"fmt"
)

// DuplicateChecker is a domain service.
//
// Whether a given Title is already in use is knowledge that does not
// naturally belong to a single Task instance: it requires querying
// across the whole collection of Tasks. Per Evans, such
// cross-aggregate logic that would otherwise force an unnatural
// responsibility onto an entity/value object belongs in a stateless
// domain service instead.
type DuplicateChecker struct {
	repo Repository
}

// NewDuplicateChecker builds a DuplicateChecker backed by repo.
func NewDuplicateChecker(repo Repository) *DuplicateChecker {
	return &DuplicateChecker{repo: repo}
}

// IsDuplicated reports whether a Task with the given title already
// exists.
func (c *DuplicateChecker) IsDuplicated(ctx context.Context, title Title) (bool, error) {
	_, err := c.repo.FindByTitle(ctx, title)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("task: check duplicated title: %w", err)
	}
	return true, nil
}
