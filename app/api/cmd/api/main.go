// Command api is the composition root of the task management sample
// application. It only wires dependencies together and manages the
// HTTP server's lifecycle; it holds no business logic itself.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/api/route"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

const (
	shutdownTimeout = 10 * time.Second

	// HTTP server timeouts (ISSUE-010 / ISSUE-024 G112). In particular,
	// ReadHeaderTimeout bounds how long a client may take to send
	// request headers, which mitigates Slowloris-style attacks (many
	// connections trickling in headers to exhaust server resources).
	// Since this server does not stream request/response bodies,
	// ReadTimeout and WriteTimeout can safely bound the whole
	// request/response. Values are kept identical to app/auth's
	// cmd/authz/main.go so the two services behave symmetrically.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

// @title        Task Management API
// @version      1.0
// @description  Task management sample API (Go, DDD-layered). This spec is generated from swag
// @description  annotations on the route package and is the contract of record for app/web codegen.
// @BasePath     /
func main() {
	if err := run(); err != nil {
		slog.Error("api: fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Persistence wiring (SPEC-005): choose infra/memory or
	// infra/postgres based on the DB_HOST/APP_ENV fail-closed contract
	// implemented by postgres.SelectMode (infra/postgres/db.go). This
	// is the only part of main.go's wiring that SPEC-005 touches; the
	// rest (domain service -> application service -> presentation) is
	// unchanged.
	e := NewEnv()
	mode, err := e.validate()
	if err != nil {
		return err
	}

	taskReader, taskWriter, closeRepo, err := newTaskRepository(ctx, mode, e.writerConfig(), e.readerConfig())
	if err != nil {
		return fmt.Errorf("api: wire persistence (mode=%s): %w", mode, err)
	}
	defer func() {
		if err := closeRepo(); err != nil {
			slog.Error("api: close persistence", "error", err)
		}
	}()

	dupChk := task.NewDuplicateChecker(taskReader)
	svc := service.NewTaskService(taskReader, taskWriter, dupChk)
	handler := route.NewRouter(svc)

	srv := newServer(":"+e.Port, handler)

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("api: server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		slog.Info("api: shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		// Wait for the ListenAndServe goroutine to finish so it is
		// never left running after run returns.
		<-serveErr
	}

	return nil
}

// newServer builds the *http.Server this process listens with, setting
// the package's four timeout constants (readHeaderTimeout/readTimeout/
// writeTimeout/idleTimeout) so the server is never left with Go's
// zero-value (unbounded) defaults. Extracted from run so it can be
// unit-tested without starting a real listener.
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// newTaskRepository builds the task.Reader/task.Writer pair selected
// by mode (postgres.SelectMode's result) and a close function the
// caller must defer to release the underlying resources (a no-op for
// infra/memory; the pool(s) postgres.OpenPair opened -- one or two
// depending on whether readerCfg equals writerCfg -- for
// infra/postgres). ctx bounds the initial Postgres connectivity check
// (postgres.OpenPair's Ping(s)); it is the server's long-lived
// shutdown context, not a separate per-call timeout, since this only
// runs once at startup.
//
// In Postgres mode, reads (task.Reader) and writes (task.Writer) are
// backed by separate *sql.DB pools (SPEC-010 R2): postgres.OpenPair
// shares a single pool between them when readerCfg == writerCfg (the
// default, DB_READER_* unset case) and opens a second, independent
// pool only when they differ. In memory mode, the same
// *memory.TaskRepository value is passed for both roles, since
// infra/memory has no notion of separate pools (SPEC-010 R5).
func newTaskRepository(ctx context.Context, mode postgres.Mode, writerCfg, readerCfg postgres.Config) (task.Reader, task.Writer, func() error, error) {
	switch mode {
	case postgres.ModePostgres:
		writerDB, readerDB, closeFn, err := postgres.OpenPair(ctx, writerCfg, readerCfg)
		if err != nil {
			return nil, nil, nil, err
		}
		slog.Info("api: persistence selected", "mode", mode, "database", writerCfg.Name)
		return postgres.NewTaskReader(readerDB), postgres.NewTaskWriter(writerDB), closeFn, nil
	default:
		slog.Info("api: persistence selected", "mode", postgres.ModeMemory)
		mem := memory.NewTaskRepository()
		return mem, mem, func() error { return nil }, nil
	}
}
