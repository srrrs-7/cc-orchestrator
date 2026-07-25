package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres/testsupport"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// intPtr returns a pointer to i, for building *int limit/offset
// arguments to TaskService.List in table-driven tests below.
func intPtr(i int) *int {
	return &i
}

// newTestService opens a real *sql.DB against the dedicated api_test
// database (testsupport.OpenTestDB), truncates the tasks table so the
// test starts from an empty store, and wires a TaskService backed by
// the real postgres.TaskRepository. As of SPEC-013, service tests run
// against real Postgres rather than a hand-written in-memory fake, so
// this package's coverage cannot silently diverge from infra/postgres's
// actual behavior.
func newTestService(t *testing.T) *service.TaskService {
	t.Helper()
	db := testsupport.OpenTestDB(t)
	testsupport.TruncateTasks(t, db)
	repo := postgres.NewTaskRepository(db)
	dupChk := task.NewDuplicateChecker(repo)
	return service.NewTaskService(repo, repo, dupChk)
}

// seedTasks inserts n distinct Tasks (titles derived from titlePrefix
// plus an index, so they satisfy the tasks.title UNIQUE constraint)
// directly into db via a real *postgres.TaskWriter. It gives paging
// tests a known row count to assert TaskListDTO.Total/Limit/Offset/
// len(Items) against, without this test package re-implementing any
// pagination math of its own (that math lives in infra/postgres's SQL).
func seedTasks(t *testing.T, db *sql.DB, n int, titlePrefix string) {
	t.Helper()
	writer := postgres.NewTaskWriter(db)
	for i := range n {
		title, err := task.NewTitle(fmt.Sprintf("%s %d", titlePrefix, i))
		if err != nil {
			t.Fatalf("NewTitle() unexpected error: %v", err)
		}
		if err := writer.Save(context.Background(), task.New(title, task.PriorityMedium)); err != nil {
			t.Fatalf("seed Save() unexpected error: %v", err)
		}
	}
}

// postgresTimestampSlack is the maximum difference SPEC-013's real-DB
// round trip can legitimately introduce between an in-process
// time.Time (nanosecond precision, and carrying a monotonic reading)
// and the same instant read back from Postgres's timestamptz column
// (microsecond precision, no monotonic reading): Postgres's encoding
// may truncate or round the sub-microsecond remainder, so the two
// values can differ by up to (but never more than) one microsecond.
// A hand-written fake previously returned the exact same struct, so
// comparing DTOs with == worked by coincidence; the real repository
// makes that comparison flake deterministically, so time fields must
// be compared with this tolerance instead.
const postgresTimestampSlack = time.Microsecond

// assertTasksRepresentSameTask compares two TaskDTOs that are expected
// to represent the same persisted Task: got is typically read back via
// TaskService.Get/List (round-tripped through Postgres) and want is
// typically the DTO TaskService.Create returned in-process. ID/Title/
// Status/Priority must match exactly; CreatedAt/UpdatedAt are compared
// within postgresTimestampSlack rather than via time.Time's == (see
// postgresTimestampSlack's doc comment).
func assertTasksRepresentSameTask(t *testing.T, got, want service.TaskDTO) {
	t.Helper()

	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
	if got.Priority != want.Priority {
		t.Errorf("Priority = %q, want %q", got.Priority, want.Priority)
	}
	assertTimeWithinSlack(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimeWithinSlack(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
}

// assertTimeWithinSlack asserts got and want represent the same instant
// within postgresTimestampSlack, using time.Time.Sub (not ==/Equal) so
// the comparison is unaffected by monotonic-clock readings or timezone
// representation differences between an in-process time.Time and one
// read back from Postgres.
func assertTimeWithinSlack(t *testing.T, field string, got, want time.Time) {
	t.Helper()
	diff := got.Sub(want)
	if diff < 0 {
		diff = -diff
	}
	if diff > postgresTimestampSlack {
		t.Errorf("%s = %v, want %v (diff %v exceeds the %v Postgres timestamptz round-trip tolerance)", field, got, want, diff, postgresTimestampSlack)
	}
}

// readerSpy implements only task.Reader's three methods
// (FindByID/FindByTitle/ListPage), delegating to a real
// *postgres.TaskReader (bound to the api_test database) and counting
// each call. It deliberately has no Save method, so passing it as
// TaskService's reader argument below is a compile-time proof
// (SPEC-010 R1) that the reader parameter's type requires nothing
// beyond task.Reader.
//
// As of SPEC-013 (R2 exception 2), this is a thin instrumentation
// decorator over the real repository rather than a hand-written
// in-memory fake: a real Postgres connection cannot itself reveal
// which port (reader vs writer) TaskService routed a call through --
// that is a property of TaskService's own wiring, not of the
// database -- so the counters here, not the data, are what this test
// observes.
type readerSpy struct {
	reader *postgres.TaskReader

	findByIDCalls    int
	findByTitleCalls int
	listPageCalls    int
}

func (r *readerSpy) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	r.findByIDCalls++
	return r.reader.FindByID(ctx, id)
}

func (r *readerSpy) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	r.findByTitleCalls++
	return r.reader.FindByTitle(ctx, title)
}

func (r *readerSpy) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	r.listPageCalls++
	return r.reader.ListPage(ctx, page)
}

// writerSpy implements only task.Writer's single method (Save),
// delegating to a real *postgres.TaskWriter bound to the same api_test
// database readerSpy's TaskReader is bound to, and counting calls. It
// deliberately has no Find*/ListPage method, so passing it as
// TaskService's writer argument is a compile-time proof that the
// writer parameter's type requires nothing beyond task.Writer.
type writerSpy struct {
	writer *postgres.TaskWriter

	saveCalls int
}

func (w *writerSpy) Save(ctx context.Context, t *task.Task) error {
	w.saveCalls++
	return w.writer.Save(ctx, t)
}

var (
	_ task.Reader = (*readerSpy)(nil)
	_ task.Writer = (*writerSpy)(nil)
)

// newSpiedService builds a TaskService whose reader and writer
// dependencies are two distinct, narrowly-typed spies both bound to
// the same real api_test database (reader == writer == db, exactly
// like the default SPEC-010 wiring when DB_READER_* is unset). This
// lets tests observe, per TaskService method, exactly which side
// (reader or writer) is called -- a routing property real Postgres
// cannot reveal on its own, since a single shared connection can't
// tell reader-role queries from writer-role queries apart (SPEC-013
// R2 exception 2).
func newSpiedService(t *testing.T) (*service.TaskService, *readerSpy, *writerSpy) {
	t.Helper()
	db := testsupport.OpenTestDB(t)
	testsupport.TruncateTasks(t, db)
	r := &readerSpy{reader: postgres.NewTaskReader(db)}
	w := &writerSpy{writer: postgres.NewTaskWriter(db)}
	dupChk := task.NewDuplicateChecker(r)
	return service.NewTaskService(r, w, dupChk), r, w
}

func TestTaskService_Create_Success(t *testing.T) {
	svc := newTestService(t)

	dto, err := svc.Create(context.Background(), "buy milk", "")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if dto.Title != "buy milk" {
		t.Errorf("Title = %q, want %q", dto.Title, "buy milk")
	}
	if dto.Status != task.StatusTodo.String() {
		t.Errorf("Status = %q, want %q", dto.Status, task.StatusTodo.String())
	}
	// R2: an unspecified (empty) priority defaults to medium.
	if dto.Priority != task.PriorityMedium.String() {
		t.Errorf("Priority = %q, want %q", dto.Priority, task.PriorityMedium.String())
	}
	if dto.ID == "" {
		t.Error("ID is empty, want non-empty")
	}
	if dto.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want set")
	}
	if dto.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want set")
	}
}

// TestTaskService_Create_Priority covers R2 (an unspecified/empty
// priority defaults to medium; an explicit priority is honored
// verbatim) and R5 (an invalid priority string is rejected with
// ErrInvalidPriority instead of silently defaulting).
func TestTaskService_Create_Priority(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		want     string
		wantErr  error
	}{
		{name: "unspecified defaults to medium (R2 boundary)", priority: "", want: task.PriorityMedium.String()},
		{name: "explicit low", priority: "low", want: task.PriorityLow.String()},
		{name: "explicit medium", priority: "medium", want: task.PriorityMedium.String()},
		{name: "explicit high", priority: "high", want: task.PriorityHigh.String()},
		{name: "invalid priority is rejected (R5)", priority: "urgent", wantErr: task.ErrInvalidPriority},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)

			dto, err := svc.Create(context.Background(), "buy milk", tt.priority)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create(_, _, %q) error = %v, want wrapping %v", tt.priority, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create(_, _, %q) unexpected error: %v", tt.priority, err)
			}
			if dto.Priority != tt.want {
				t.Errorf("Priority = %q, want %q", dto.Priority, tt.want)
			}
		})
	}
}

func TestTaskService_Create_DuplicateTitle(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.Create(context.Background(), "buy milk", ""); err != nil {
		t.Fatalf("setup Create() unexpected error: %v", err)
	}

	_, err := svc.Create(context.Background(), "buy milk", "")
	if !errors.Is(err, task.ErrDuplicateTitle) {
		t.Fatalf("Create() error = %v, want wrapping %v", err, task.ErrDuplicateTitle)
	}
}

func TestTaskService_Create_InvalidTitle(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		wantErr error
	}{
		{name: "empty title", title: "", wantErr: task.ErrEmptyTitle},
		{name: "whitespace only title", title: "   ", wantErr: task.ErrEmptyTitle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)

			_, err := svc.Create(context.Background(), tt.title, "")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create(%q) error = %v, want wrapping %v", tt.title, err, tt.wantErr)
			}
		})
	}
}

func TestTaskService_Get(t *testing.T) {
	t.Run("existing task is found", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		got, err := svc.Get(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		assertTasksRepresentSameTask(t, got, created)
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc := newTestService(t)

		_, err := svc.Get(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Get() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("empty id is invalid", func(t *testing.T) {
		svc := newTestService(t)

		_, err := svc.Get(context.Background(), "")
		if !errors.Is(err, task.ErrInvalidID) {
			t.Fatalf("Get() error = %v, want wrapping %v", err, task.ErrInvalidID)
		}
	})
}

func TestTaskService_List(t *testing.T) {
	svc := newTestService(t)

	got, err := svc.List(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(got.Items) != 0 {
		t.Fatalf("List() on empty repo = %d items, want 0", len(got.Items))
	}
	if got.Total != 0 {
		t.Fatalf("List() Total on empty repo = %d, want 0", got.Total)
	}

	if _, err := svc.Create(context.Background(), "buy milk", ""); err != nil {
		t.Fatalf("setup Create() unexpected error: %v", err)
	}
	if _, err := svc.Create(context.Background(), "walk dog", ""); err != nil {
		t.Fatalf("setup Create() unexpected error: %v", err)
	}

	got, err = svc.List(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("List() = %d items, want 2", len(got.Items))
	}
	if got.Total != 2 {
		t.Fatalf("List() Total = %d, want 2", got.Total)
	}
}

// TestTaskService_List_PagingAppliedAndEchoed covers R1 (unspecified
// limit/offset default to 20/0)/R2 (TaskListDTO.Total/Limit/Offset
// echo the values the server actually applied, and Total is the
// repository's own total independent of len(items) returned for the
// page)/R3 (a limit above task.MaxLimit is clamped, and the clamped
// value -- not the raw request -- is both forwarded to the
// repository and echoed in the DTO).
//
// As of SPEC-013, this seeds a known row count into a real api_test
// database and asserts black-box against TaskListDTO (Total/Limit/
// Offset/len(Items)), rather than inspecting a stub's recorded
// task.Page argument directly: infra/postgres's real SQL, not a
// hand-written fake, is what actually applies limit/offset now.
func TestTaskService_List_PagingAppliedAndEchoed(t *testing.T) {
	tests := []struct {
		name       string
		seedCount  int
		limit      *int
		offset     *int
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "unspecified limit/offset default to 20/0 and echo the defaults (R1)",
			seedCount:  3,
			limit:      nil,
			offset:     nil,
			wantLimit:  task.DefaultLimit,
			wantOffset: 0,
		},
		{
			name:       "explicit limit/offset are forwarded to the repository and echoed (R2)",
			seedCount:  15,
			limit:      intPtr(5),
			offset:     intPtr(10),
			wantLimit:  5,
			wantOffset: 10,
		},
		{
			name:       "limit above MaxLimit is clamped to 100 before reaching the repository, and the clamp is echoed (R3)",
			seedCount:  task.MaxLimit + 5,
			limit:      intPtr(1000),
			offset:     nil,
			wantLimit:  task.MaxLimit,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testsupport.OpenTestDB(t)
			testsupport.TruncateTasks(t, db)
			seedTasks(t, db, tt.seedCount, "paging task")

			repo := postgres.NewTaskRepository(db)
			dupChk := task.NewDuplicateChecker(repo)
			svc := service.NewTaskService(repo, repo, dupChk)

			got, err := svc.List(context.Background(), tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}

			// Total must equal the seeded row count, independent of the
			// limit/offset window applied to Items -- pinning that
			// TaskService never recomputes it from the returned page.
			if got.Total != tt.seedCount {
				t.Errorf("Total = %d, want %d (must equal the store's full row count, independent of the page window)", got.Total, tt.seedCount)
			}
			if got.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", got.Limit, tt.wantLimit)
			}
			if got.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", got.Offset, tt.wantOffset)
			}

			// The already defaulted/clamped Page -- not the raw
			// caller-supplied limit/offset -- must be what actually
			// reached the repository: len(Items) proves this
			// black-box, since a raw (unclamped) limit of 1000 forwarded
			// to Postgres would return more than task.MaxLimit rows.
			wantItems := min(tt.wantLimit, max(tt.seedCount-tt.wantOffset, 0))
			if len(got.Items) != wantItems {
				t.Errorf("len(Items) = %d, want %d (= min(limit, total-offset))", len(got.Items), wantItems)
			}
		})
	}
}

// TestTaskService_List_InvalidLimitOffset covers R3's rejection path:
// an out-of-range limit/offset is rejected by task.NewPage before
// TaskService.List ever calls the repository, so no repository call
// should be observed. It uses readerSpy's call counter (rather than a
// hand-written fake) since ListPage is a pure read with no persisted
// side effect this test package could otherwise observe.
func TestTaskService_List_InvalidLimitOffset(t *testing.T) {
	tests := []struct {
		name    string
		limit   *int
		offset  *int
		wantErr error
	}{
		{name: "limit less than 1 is rejected", limit: intPtr(0), offset: nil, wantErr: task.ErrInvalidLimit},
		{name: "negative limit is rejected", limit: intPtr(-5), offset: nil, wantErr: task.ErrInvalidLimit},
		{name: "negative offset is rejected", limit: nil, offset: intPtr(-1), wantErr: task.ErrInvalidOffset},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, r, _ := newSpiedService(t)

			_, err := svc.List(context.Background(), tt.limit, tt.offset)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("List() error = %v, want wrapping %v", err, tt.wantErr)
			}
			if r.listPageCalls != 0 {
				t.Errorf("reader.listPageCalls = %d, want 0 (validation must short-circuit before the repository call)", r.listPageCalls)
			}
		})
	}
}

func TestTaskService_Start(t *testing.T) {
	t.Run("todo transitions to doing", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		got, err := svc.Start(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("Start() unexpected error: %v", err)
		}
		if got.Status != task.StatusDoing.String() {
			t.Errorf("Status = %q, want %q", got.Status, task.StatusDoing.String())
		}
	})

	t.Run("invalid transition error propagates", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}
		if _, err := svc.Start(context.Background(), created.ID); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}

		_, err = svc.Start(context.Background(), created.ID)

		var transitionErr *task.TransitionError
		if !errors.As(err, &transitionErr) {
			t.Fatalf("Start() error = %v, want *task.TransitionError", err)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc := newTestService(t)

		_, err := svc.Start(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Start() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

func TestTaskService_Complete(t *testing.T) {
	t.Run("doing transitions to done", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}
		if _, err := svc.Start(context.Background(), created.ID); err != nil {
			t.Fatalf("setup Start() unexpected error: %v", err)
		}

		got, err := svc.Complete(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("Complete() unexpected error: %v", err)
		}
		if got.Status != task.StatusDone.String() {
			t.Errorf("Status = %q, want %q", got.Status, task.StatusDone.String())
		}
	})

	t.Run("invalid transition from todo propagates", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		_, err = svc.Complete(context.Background(), created.ID)

		var transitionErr *task.TransitionError
		if !errors.As(err, &transitionErr) {
			t.Fatalf("Complete() error = %v, want *task.TransitionError", err)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc := newTestService(t)

		_, err := svc.Complete(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Complete() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

// TestTaskService_ChangePriority covers R3 (a valid priority change
// is persisted and reflected in the returned DTO, without touching
// status) and R5 (an invalid priority value is rejected) plus the
// not-found boundary shared with Start/Complete.
func TestTaskService_ChangePriority(t *testing.T) {
	t.Run("changes priority without touching status", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "low")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		got, err := svc.ChangePriority(context.Background(), created.ID, "high")
		if err != nil {
			t.Fatalf("ChangePriority() unexpected error: %v", err)
		}
		if got.Priority != task.PriorityHigh.String() {
			t.Errorf("Priority = %q, want %q", got.Priority, task.PriorityHigh.String())
		}
		if got.Status != task.StatusTodo.String() {
			t.Errorf("Status = %q, want unchanged %q", got.Status, task.StatusTodo.String())
		}
	})

	t.Run("invalid priority value is rejected (R5)", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		_, err = svc.ChangePriority(context.Background(), created.ID, "urgent")
		if !errors.Is(err, task.ErrInvalidPriority) {
			t.Fatalf("ChangePriority() error = %v, want wrapping %v", err, task.ErrInvalidPriority)
		}
	})

	t.Run("empty priority value is rejected (R5, strict boundary)", func(t *testing.T) {
		svc := newTestService(t)
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		_, err = svc.ChangePriority(context.Background(), created.ID, "")
		if !errors.Is(err, task.ErrInvalidPriority) {
			t.Fatalf("ChangePriority() error = %v, want wrapping %v", err, task.ErrInvalidPriority)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc := newTestService(t)

		_, err := svc.ChangePriority(context.Background(), task.NewID().String(), "high")
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("ChangePriority() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

// TestTaskService_RoutesReaderAndWriter is SPEC-010 R2's service-level
// proof: every read-only operation (Get, List, and the FindByID lookup
// inside Start/Complete/ChangePriority, plus DuplicateChecker's
// FindByTitle pre-check inside Create) must go through the reader
// dependency, and every state-changing operation (Create's Save, and
// the Save inside Start/Complete/ChangePriority) must go through the
// writer dependency -- never the other way around. readerSpy/writerSpy
// cannot silently substitute for one another (neither type has the
// other's methods), so this also compile-time-proves TaskService's
// reader/writer parameters are the narrow task.Reader/task.Writer
// ports, not the full task.Repository.
func TestTaskService_RoutesReaderAndWriter(t *testing.T) {
	svc, r, w := newSpiedService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "buy milk", "")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if w.saveCalls != 1 {
		t.Errorf("after Create(): writer.saveCalls = %d, want 1", w.saveCalls)
	}
	if r.findByTitleCalls != 1 {
		t.Errorf("after Create(): reader.findByTitleCalls = %d, want 1 (DuplicateChecker's pre-check must use the reader)", r.findByTitleCalls)
	}

	if _, err := svc.Get(ctx, created.ID); err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if r.findByIDCalls != 1 {
		t.Errorf("after Get(): reader.findByIDCalls = %d, want 1", r.findByIDCalls)
	}
	if w.saveCalls != 1 {
		t.Errorf("Get() must never call the writer; writer.saveCalls = %d, want unchanged 1", w.saveCalls)
	}

	if _, err := svc.List(ctx, nil, nil); err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if r.listPageCalls != 1 {
		t.Errorf("after List(): reader.listPageCalls = %d, want 1", r.listPageCalls)
	}

	if _, err := svc.Start(ctx, created.ID); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	if r.findByIDCalls != 2 {
		t.Errorf("after Start(): reader.findByIDCalls = %d, want 2 (Start must look the Task up via the reader)", r.findByIDCalls)
	}
	if w.saveCalls != 2 {
		t.Errorf("after Start(): writer.saveCalls = %d, want 2 (Start must persist the transition via the writer)", w.saveCalls)
	}

	if _, err := svc.ChangePriority(ctx, created.ID, "high"); err != nil {
		t.Fatalf("ChangePriority() unexpected error: %v", err)
	}
	if r.findByIDCalls != 3 {
		t.Errorf("after ChangePriority(): reader.findByIDCalls = %d, want 3", r.findByIDCalls)
	}
	if w.saveCalls != 3 {
		t.Errorf("after ChangePriority(): writer.saveCalls = %d, want 3", w.saveCalls)
	}

	if _, err := svc.Complete(ctx, created.ID); err != nil {
		t.Fatalf("Complete() unexpected error: %v", err)
	}
	if r.findByIDCalls != 4 {
		t.Errorf("after Complete(): reader.findByIDCalls = %d, want 4", r.findByIDCalls)
	}
	if w.saveCalls != 4 {
		t.Errorf("after Complete(): writer.saveCalls = %d, want 4", w.saveCalls)
	}

	// Across the whole scenario, FindByTitle must only ever have been
	// called once (Create's duplicate check) -- List/Get/Start/
	// Complete/ChangePriority never touch it.
	if r.findByTitleCalls != 1 {
		t.Errorf("final reader.findByTitleCalls = %d, want 1", r.findByTitleCalls)
	}
}
