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
	"github.com/srrrs-7/cc-orchestrator/app/api/infra/jwt"
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
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 RS256 JWT access token issued by the auth server. Format: "Bearer <token>".
func main() {
	if err := run(); err != nil {
		slog.Error("api: fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Persistence wiring (SPEC-005 / SPEC-011): Postgres is the only
	// backend. validate() enforces fail-closed via Config.Validate
	// (DB_HOST/DB_NAME/DB_USER/DB_PASSWORD required); the process will
	// not start if any of those are missing.
	e := NewEnv()
	if err := e.validate(); err != nil {
		return err
	}

	taskReader, taskWriter, closeRepo, err := newTaskRepository(ctx, e.writerConfig(), e.readerConfig())
	if err != nil {
		return fmt.Errorf("api: wire persistence: %w", err)
	}
	defer func() {
		if err := closeRepo(); err != nil {
			slog.Error("api: close persistence", "error", err)
		}
	}()

	dupChk := task.NewDuplicateChecker(taskReader)
	svc := service.NewTaskService(taskReader, taskWriter, dupChk)
	taskHandler := route.NewRouter(svc)

	// Wire Bearer JWT auth middleware when both AUTH_ISSUER and
	// AUTH_JWKS_URL are set. When they are unset (e.g. local dev
	// without an auth server) the middleware is skipped and all
	// requests pass through unauthenticated.
	var apiHandler = taskHandler
	if e.authEnabled() {
		slog.Info("api: JWT auth enabled", "issuer", e.AuthIssuer, "jwks_url", e.AuthJWKSURL)
		verifier := jwt.NewVerifier(e.AuthJWKSURL, e.AuthIssuer)
		apiHandler = route.AuthMiddleware(verifier, taskHandler)
	} else {
		slog.Warn("api: JWT auth disabled (AUTH_ISSUER and AUTH_JWKS_URL not set)")
	}

	// GET /health is mounted outside auth middleware for liveness probes.
	rootMux := http.NewServeMux()
	route.RegisterHealthRoute(rootMux)
	rootMux.Handle("/", apiHandler)

	srv := newServer(":"+e.Port, rootMux)

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

// newTaskRepository opens writer and reader connection pools via
// postgres.OpenPair (SPEC-010 R2) and returns task.Reader/task.Writer
// backed by those pools, plus a closeFn the caller must defer to
// release them. ctx bounds the initial connectivity check (Ping).
//
// When readerCfg equals writerCfg (the default when DB_READER_* is
// unset), OpenPair shares a single *sql.DB pool; only when they differ
// does it open a second, independent reader pool.
func newTaskRepository(ctx context.Context, writerCfg, readerCfg postgres.Config) (task.Reader, task.Writer, func() error, error) {
	writerDB, readerDB, closeFn, err := postgres.OpenPair(ctx, writerCfg, readerCfg)
	if err != nil {
		return nil, nil, nil, err
	}
	slog.Info("api: persistence initialized", "database", writerCfg.Name)
	return postgres.NewTaskReader(readerDB), postgres.NewTaskWriter(writerDB), closeFn, nil
}
