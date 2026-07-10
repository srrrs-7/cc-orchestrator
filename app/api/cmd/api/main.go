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

	repo, closeRepo, err := newTaskRepository(ctx, mode, e.dbConfig())
	if err != nil {
		return fmt.Errorf("api: wire persistence (mode=%s): %w", mode, err)
	}
	defer func() {
		if err := closeRepo(); err != nil {
			slog.Error("api: close persistence", "error", err)
		}
	}()

	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, dupChk)
	handler := route.NewRouter(svc)

	srv := &http.Server{
		Addr:    ":" + e.Port,
		Handler: handler,
	}

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

// newTaskRepository builds the task.Repository selected by mode
// (postgres.SelectMode's result) and a close function the caller must
// defer to release the underlying resources (a no-op for
// infra/memory; db.Close() -- releasing the connection pool -- for
// infra/postgres). ctx bounds the initial Postgres connectivity check
// (postgres.Open's Ping); it is the server's long-lived shutdown
// context, not a separate per-call timeout, since this only runs once
// at startup.
func newTaskRepository(ctx context.Context, mode postgres.Mode, cfg postgres.Config) (task.Repository, func() error, error) {
	switch mode {
	case postgres.ModePostgres:
		db, err := postgres.Open(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
		slog.Info("api: persistence selected", "mode", mode, "database", cfg.Name)
		return postgres.NewTaskRepository(db), db.Close, nil
	default:
		slog.Info("api: persistence selected", "mode", postgres.ModeMemory)
		return memory.NewTaskRepository(), func() error { return nil }, nil
	}
}
