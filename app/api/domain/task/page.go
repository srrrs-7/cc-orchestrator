package task

// DefaultLimit is the number of items a Page applies when the caller
// does not specify a limit.
const DefaultLimit = 20

// MaxLimit is the upper bound a caller-supplied limit is clamped to.
// A limit greater than MaxLimit is silently reduced to MaxLimit
// rather than rejected (see NewPage).
const MaxLimit = 100

// Page is a value object representing a validated, clamped
// limit/offset pair for a paginated Repository query (SPEC-008). It
// centralizes the business rule shared by every Repository
// implementation and the route layer: a nil limit/offset defaults to
// DefaultLimit/0, a limit above MaxLimit is clamped, and a limit < 1
// or a negative offset is rejected outright. Page is immutable and
// guarantees these invariants at construction time via NewPage.
type Page struct {
	limit  int
	offset int
}

// NewPage builds a Page from an optional limit and offset. A nil
// limit defaults to DefaultLimit; a nil offset defaults to 0. A limit
// greater than MaxLimit is clamped to MaxLimit (not an error). It
// returns a *ValidationError wrapping ErrInvalidLimit if the
// (defaulted) limit is less than 1, or wrapping ErrInvalidOffset if
// the (defaulted) offset is negative.
func NewPage(limit, offset *int) (Page, error) {
	l := DefaultLimit
	if limit != nil {
		l = *limit
	}
	if l < 1 {
		return Page{}, &ValidationError{Msg: "limit must be at least 1", Err: ErrInvalidLimit}
	}
	if l > MaxLimit {
		l = MaxLimit
	}

	o := 0
	if offset != nil {
		o = *offset
	}
	if o < 0 {
		return Page{}, &ValidationError{Msg: "offset must be at least 0", Err: ErrInvalidOffset}
	}

	return Page{limit: l, offset: o}, nil
}

// Limit returns the page's applied limit (after default/clamp).
func (p Page) Limit() int {
	return p.limit
}

// Offset returns the page's applied offset (after default).
func (p Page) Offset() int {
	return p.offset
}
