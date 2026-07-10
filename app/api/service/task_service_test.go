package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

// fakeRepository is an in-memory task.Repository fake used to
// exercise TaskService without depending on the infra layer.
type fakeRepository struct {
	mu    sync.Mutex
	tasks map[task.ID]*task.Task
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{tasks: make(map[task.ID]*task.Task)}
}

func (f *fakeRepository) Save(ctx context.Context, t *task.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks[t.ID()] = t
	return nil
}

func (f *fakeRepository) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, task.ErrNotFound
	}
	return t, nil
}

func (f *fakeRepository) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tasks {
		if t.Title() == title {
			return t, nil
		}
	}
	return nil, task.ErrNotFound
}

func (f *fakeRepository) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := make([]*task.Task, 0, len(f.tasks))
	for _, t := range f.tasks {
		all = append(all, t)
	}
	slices.SortFunc(all, func(a, b *task.Task) int {
		if c := a.CreatedAt().Compare(b.CreatedAt()); c != 0 {
			return c
		}
		return strings.Compare(a.ID().String(), b.ID().String())
	})

	total := len(all)
	start := min(page.Offset(), total)
	end := min(start+page.Limit(), total)
	return all[start:end], total, nil
}

// intPtr returns a pointer to i, for building *int limit/offset
// arguments to TaskService.List in table-driven tests below.
func intPtr(i int) *int {
	return &i
}

func newTestService() (*service.TaskService, *fakeRepository) {
	repo := newFakeRepository()
	dupChk := task.NewDuplicateChecker(repo)
	// SPEC-010: NewTaskService takes reader and writer separately
	// (task.Reader / task.Writer). fakeRepository implements every
	// method of both (a single in-memory-style store, mirroring
	// infra/memory's R5 requirement), so the same value satisfies
	// both roles here.
	return service.NewTaskService(repo, repo, dupChk), repo
}

// stubListPageRepository wraps a *fakeRepository (inheriting its
// Save/FindByID/FindByTitle) but overrides ListPage to return a
// caller-configured items/total pair verbatim, decoupled from any
// particular repository's own pagination math. It exists to test
// TaskService.List's wiring in isolation (SPEC-008 R1-R3): that it
// forwards task.NewPage's defaulted/clamped Page to the repository
// and echoes the repository's own total (which need not equal
// len(items), e.g. a real backend applying limit/offset over many
// more rows) into TaskListDTO, and it records the Page it was called
// with so tests can assert what TaskService actually passed through.
type stubListPageRepository struct {
	*fakeRepository
	items []*task.Task
	total int

	gotPage      task.Page
	listPageCall int
}

func (s *stubListPageRepository) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	s.gotPage = page
	s.listPageCall++
	return s.items, s.total, nil
}

func newStubListPageService(items []*task.Task, total int) (*service.TaskService, *stubListPageRepository) {
	stub := &stubListPageRepository{fakeRepository: newFakeRepository(), items: items, total: total}
	dupChk := task.NewDuplicateChecker(stub)
	return service.NewTaskService(stub, stub, dupChk), stub
}

// readerSpy implements only task.Reader's three methods
// (FindByID/FindByTitle/ListPage), delegating to a shared
// *fakeRepository and counting each call. It deliberately has no Save
// method, so passing it as TaskService's reader argument below is a
// compile-time proof (SPEC-010 R1) that the reader parameter's type
// requires nothing beyond task.Reader.
type readerSpy struct {
	repo *fakeRepository

	findByIDCalls    int
	findByTitleCalls int
	listPageCalls    int
}

func (r *readerSpy) FindByID(ctx context.Context, id task.ID) (*task.Task, error) {
	r.findByIDCalls++
	return r.repo.FindByID(ctx, id)
}

func (r *readerSpy) FindByTitle(ctx context.Context, title task.Title) (*task.Task, error) {
	r.findByTitleCalls++
	return r.repo.FindByTitle(ctx, title)
}

func (r *readerSpy) ListPage(ctx context.Context, page task.Page) ([]*task.Task, int, error) {
	r.listPageCalls++
	return r.repo.ListPage(ctx, page)
}

// writerSpy implements only task.Writer's single method (Save),
// delegating to the same shared *fakeRepository readerSpy wraps, and
// counting calls. It deliberately has no Find*/ListPage method, so
// passing it as TaskService's writer argument is a compile-time proof
// that the writer parameter's type requires nothing beyond
// task.Writer.
type writerSpy struct {
	repo *fakeRepository

	saveCalls int
}

func (w *writerSpy) Save(ctx context.Context, t *task.Task) error {
	w.saveCalls++
	return w.repo.Save(ctx, t)
}

// newSpiedService builds a TaskService whose reader and writer
// dependencies are two distinct, narrowly-typed spies sharing one
// underlying store (so a Task saved via the writer is visible to the
// reader, exactly like a single physical database observed through two
// separate connection pools -- SPEC-010's writer/reader pool split).
// This lets TestTaskService_RoutesReaderAndWriter observe, per
// TaskService method, exactly which side (reader or writer) is called.
func newSpiedService() (*service.TaskService, *readerSpy, *writerSpy) {
	shared := newFakeRepository()
	r := &readerSpy{repo: shared}
	w := &writerSpy{repo: shared}
	dupChk := task.NewDuplicateChecker(r)
	return service.NewTaskService(r, w, dupChk), r, w
}

func TestTaskService_Create_Success(t *testing.T) {
	svc, _ := newTestService()

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
			svc, _ := newTestService()

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
	svc, _ := newTestService()

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
			svc, _ := newTestService()

			_, err := svc.Create(context.Background(), tt.title, "")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create(%q) error = %v, want wrapping %v", tt.title, err, tt.wantErr)
			}
		})
	}
}

func TestTaskService_Get(t *testing.T) {
	t.Run("existing task is found", func(t *testing.T) {
		svc, _ := newTestService()
		created, err := svc.Create(context.Background(), "buy milk", "")
		if err != nil {
			t.Fatalf("setup Create() unexpected error: %v", err)
		}

		got, err := svc.Get(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		if got != created {
			t.Errorf("Get() = %+v, want %+v", got, created)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.Get(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Get() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})

	t.Run("empty id is invalid", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.Get(context.Background(), "")
		if !errors.Is(err, task.ErrInvalidID) {
			t.Fatalf("Get() error = %v, want wrapping %v", err, task.ErrInvalidID)
		}
	})
}

func TestTaskService_List(t *testing.T) {
	svc, _ := newTestService()

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
func TestTaskService_List_PagingAppliedAndEchoed(t *testing.T) {
	title, err := task.NewTitle("buy milk")
	if err != nil {
		t.Fatalf("NewTitle() unexpected error: %v", err)
	}
	items := []*task.Task{task.New(title, task.PriorityMedium)}

	tests := []struct {
		name       string
		limit      *int
		offset     *int
		stubTotal  int
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "unspecified limit/offset default to 20/0 and echo the defaults (R1)",
			limit:      nil,
			offset:     nil,
			stubTotal:  3,
			wantLimit:  task.DefaultLimit,
			wantOffset: 0,
		},
		{
			name:       "explicit limit/offset are forwarded to the repository and echoed (R2)",
			limit:      intPtr(5),
			offset:     intPtr(10),
			stubTotal:  42,
			wantLimit:  5,
			wantOffset: 10,
		},
		{
			name:       "limit above MaxLimit is clamped to 100 before reaching the repository, and the clamp is echoed (R3)",
			limit:      intPtr(1000),
			offset:     nil,
			stubTotal:  250,
			wantLimit:  task.MaxLimit,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, stub := newStubListPageService(items, tt.stubTotal)

			got, err := svc.List(context.Background(), tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}

			// Total echoes the repository's own total verbatim, even
			// though it does not equal len(items) here -- pinning that
			// TaskService never recomputes it from the returned page.
			if got.Total != tt.stubTotal {
				t.Errorf("Total = %d, want %d (must echo the repository's total, independent of items returned)", got.Total, tt.stubTotal)
			}
			if got.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", got.Limit, tt.wantLimit)
			}
			if got.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", got.Offset, tt.wantOffset)
			}
			if len(got.Items) != len(items) {
				t.Errorf("len(Items) = %d, want %d", len(got.Items), len(items))
			}

			// The repository must receive the already defaulted/clamped
			// Page, not the raw caller-supplied limit/offset.
			if stub.listPageCall != 1 {
				t.Fatalf("repo.ListPage called %d times, want 1", stub.listPageCall)
			}
			if stub.gotPage.Limit() != tt.wantLimit {
				t.Errorf("repo received Page.Limit() = %d, want %d", stub.gotPage.Limit(), tt.wantLimit)
			}
			if stub.gotPage.Offset() != tt.wantOffset {
				t.Errorf("repo received Page.Offset() = %d, want %d", stub.gotPage.Offset(), tt.wantOffset)
			}
		})
	}
}

// TestTaskService_List_InvalidLimitOffset covers R3's rejection path:
// an out-of-range limit/offset is rejected by task.NewPage before
// TaskService.List ever calls the repository, so no repository call
// should be observed.
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
			svc, stub := newStubListPageService(nil, 0)

			_, err := svc.List(context.Background(), tt.limit, tt.offset)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("List() error = %v, want wrapping %v", err, tt.wantErr)
			}
			if stub.listPageCall != 0 {
				t.Errorf("repo.ListPage called %d times, want 0 (validation must short-circuit before the repository call)", stub.listPageCall)
			}
		})
	}
}

func TestTaskService_Start(t *testing.T) {
	t.Run("todo transitions to doing", func(t *testing.T) {
		svc, _ := newTestService()
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
		svc, _ := newTestService()
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
		svc, _ := newTestService()

		_, err := svc.Start(context.Background(), task.NewID().String())
		if !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("Start() error = %v, want wrapping %v", err, task.ErrNotFound)
		}
	})
}

func TestTaskService_Complete(t *testing.T) {
	t.Run("doing transitions to done", func(t *testing.T) {
		svc, _ := newTestService()
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
		svc, _ := newTestService()
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
		svc, _ := newTestService()

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
		svc, _ := newTestService()
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
		svc, _ := newTestService()
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
		svc, _ := newTestService()
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
		svc, _ := newTestService()

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
	svc, r, w := newSpiedService()
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
