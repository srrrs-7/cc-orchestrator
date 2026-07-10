package task

import "context"

// Reader is the query half of the Task aggregate's persistence
// boundary (SPEC-010 R1). It is defined in the domain layer
// (dependency inversion): the domain declares what it needs, and the
// infrastructure layer provides a concrete implementation, keeping
// the domain independent of any storage technology.
//
// Reader exists so that call sites which only ever read Tasks (e.g.
// DuplicateChecker) can depend on the narrowest interface that
// satisfies them, and so that the composition root can route Reader
// implementations to a different physical connection pool (a reader
// pool, potentially a read replica) than Writer implementations.
//
// FindByID and FindByTitle return ErrNotFound when no matching Task
// exists.
type Reader interface {
	FindByID(ctx context.Context, id ID) (*Task, error)
	FindByTitle(ctx context.Context, title Title) (*Task, error)

	// ListPage returns a stable-ordered (created_at, id ascending)
	// page of Tasks per page's limit/offset, along with total: the
	// number of Tasks in the store independent of the page window
	// (SPEC-008 R2). An offset at or beyond total yields an empty
	// items slice, not an error.
	ListPage(ctx context.Context, page Page) (items []*Task, total int, err error)
}

// Writer is the command half of the Task aggregate's persistence
// boundary (SPEC-010 R1). See Reader's doc comment for the rationale
// behind splitting persistence into two interfaces.
type Writer interface {
	Save(ctx context.Context, t *Task) error
}

// Repository is the full persistence boundary for the Task
// aggregate: the composition of Reader and Writer. It exists for
// consumers that legitimately need both roles (e.g. the shared
// repository contract test, and infra implementations that back a
// single physical store for both reads and writes) and is kept
// additive on top of Reader/Writer so that every pre-existing
// Repository implementation and consumer continues to compile
// unchanged (SPEC-010 R6).
type Repository interface {
	Reader
	Writer
}
