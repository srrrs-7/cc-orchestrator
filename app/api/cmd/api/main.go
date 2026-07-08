// Command api is the composition root of the task management sample
// application. It only wires dependencies together and manages the
// HTTP server's lifecycle; it holds no business logic itself.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/api/domain/task"
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/api/route"
	"github.com/srrrs-7/cc-orchestrator/app/api/service"
)

const (
	defaultPort     = "8080"
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

	// Wiring: infra -> domain service -> application service -> presentation.
	repo := memory.NewTaskRepository()
	dupChk := task.NewDuplicateChecker(repo)
	svc := service.NewTaskService(repo, dupChk)
	handler := route.NewRouter(svc)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	srv := &http.Server{
		Addr:    ":" + port,
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
