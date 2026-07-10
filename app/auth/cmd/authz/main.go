// Command authz is the composition root of the OAuth 2.0 authorization
// server / OpenID Provider sample application. It only wires
// dependencies together and manages the HTTP server's lifecycle; it
// holds no business logic itself.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/route"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

const (
	rsaKeyBits      = 2048
	shutdownTimeout = 10 * time.Second

	// HTTP server timeouts. In particular, ReadHeaderTimeout bounds how
	// long a client may take to send request headers, which mitigates
	// Slowloris-style attacks (many connections trickling in headers
	// to exhaust server resources) against an authorization server
	// that, by design, faces every relying-party client. Since this
	// server does not stream request/response bodies, ReadTimeout and
	// WriteTimeout can safely bound the whole request/response.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second

	// Seed data for this demo authorization server. None of these are
	// secret: a client_id, a registered redirect_uri, a username and
	// a subject id are all public identifiers by design (RFC 6749
	// 2.1/2.2). They are documented in README.md. The RSA signing key
	// and the demo user's password *are* secrets and are generated
	// fresh at process startup instead (see run() / buildDemoUser()).
	demoClientID    = "demo-client"
	demoRedirectURI = "http://localhost:3000/callback"
	demoUsername    = "demo-user"
	demoUserID      = "demo-user-id"
	demoUserName    = "Demo User"
	demoUserEmail   = "demo-user@example.com"
)

func main() {
	if err := run(); err != nil {
		slog.Error("authz: fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	e := NewEnv()
	mode, err := e.validate()
	if err != nil {
		return err // already wrapped "authz: validate env: ..."
	}
	// OIDC Discovery 1.0 requires a production issuer to use https;
	// defaultIssuer's http scheme is only appropriate for local
	// development. A production deployment MUST set ISSUER to an
	// https URL (see README.md "issuer").

	// RSA signing key: generated fresh in memory every time the
	// process starts. It is never written to disk, logged, or
	// otherwise persisted, so restarting the process invalidates
	// every token issued by the previous instance (see README.md).
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return fmt.Errorf("authz: generate rsa key: %w", err)
	}
	kid, err := jwt.ComputeKeyID(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("authz: compute key id: %w", err)
	}
	signer := jwt.NewSigner(privateKey, kid)
	verifier := jwt.NewVerifier(&privateKey.PublicKey)
	keyProvider := jwt.NewKeyProvider(&privateKey.PublicKey, kid)

	// Repositories + demo seed data (SPEC-005: Postgres or in-memory,
	// chosen by setupPersistence based on DB_HOST/APP_ENV -- see its
	// doc comment for the fail-closed selection contract). SPEC-010:
	// in Postgres mode, setupPersistence opens a writer/reader pool
	// pair via postgres.OpenPair and wires each repository to the
	// pool the "auth の correctness-critical read の配置" table
	// (docs/plans/SPEC-010-plan.md) assigns it.
	clientRepo, userRepo, authCodeRepo, refreshTokenRepo, closePersistence, err := setupPersistence(ctx, mode, e.writerConfig(), e.readerConfig())
	if err != nil {
		return fmt.Errorf("authz: setup persistence: %w", err)
	}
	defer func() {
		if err := closePersistence(); err != nil {
			slog.Error("authz: close persistence", "error", err)
		}
	}()

	defaultUsername, err := user.NewUsername(demoUsername)
	if err != nil {
		return fmt.Errorf("authz: build default username: %w", err)
	}

	// Wiring: infra -> application service -> presentation.
	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, refreshTokenRepo, signer, e.Issuer, defaultUsername)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, e.Issuer)
	discoverySvc := service.NewDiscoveryService(e.Issuer, keyProvider)
	handler := route.NewRouter(authSvc, userInfoSvc, discoverySvc)

	srv := &http.Server{
		Addr:              ":" + e.Port,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("authz: server starting", "addr", srv.Addr, "issuer", e.Issuer, "kid", kid)
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
		slog.Info("authz: shutdown signal received")

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

// setupPersistence is SPEC-005's persistence composition block: it
// wires infra/postgres or infra/memory depending on the mode/cfg the
// caller provides. mode is resolved once, up front, by Env.validate
// (env.go) via postgres.SelectMode's fail-closed contract --
//
//   - DB_HOST set        -> Postgres, regardless of APP_ENV
//   - DB_HOST unset,
//     APP_ENV=local|test -> in-memory (this sample's original behavior)
//   - DB_HOST unset,
//     any other APP_ENV  -> a wrapped error (no silent memory
//     fallback; this includes an unset APP_ENV and
//     APP_ENV=production)
//
// so a production deployment that forgot to configure DB_HOST fails
// to start instead of silently running on non-durable, single-instance
// in-memory storage (docs/plans/SPEC-005-plan.md §0 "切替の env / DSN
// / 本番必須強制").
//
// writerCfg/readerCfg are Env.writerConfig()/Env.readerConfig()
// (SPEC-010): in Postgres mode they are passed to postgres.OpenPair,
// which opens a single shared *sql.DB when readerCfg == writerCfg
// (the DB_READER_* -unset default) or two independent pools
// otherwise. Repository construction then follows
// docs/plans/SPEC-010-plan.md's "auth の correctness-critical read の
// 配置" table: client/user (seeded once at startup, never written at
// runtime) are wired to the reader pool, while authcode/refreshtoken
// (single-use tokens whose read-after-write correctness must not be
// exposed to replica lag) stay pinned to the writer pool for both
// reads and writes.
//
// It returns the four repositories as their domain-declared
// interfaces (client.Repository / user.Repository /
// authcode.Repository / refreshtoken.Repository -- the last added by
// SPEC-006) -- the rest of run() never needs to know which backend is
// in play -- plus a closePersistence func the caller MUST defer-call
// during shutdown to release any pooled Postgres connections (a no-op
// for the in-memory backend, per infra/postgres.Open's
// ctx-bound-ping-only lifecycle contract).
func setupPersistence(ctx context.Context, mode postgres.Mode, writerCfg, readerCfg postgres.Config) (client.Repository, user.Repository, authcode.Repository, refreshtoken.Repository, func() error, error) {
	noopClose := func() error { return nil }

	switch mode {
	case postgres.ModeMemory:
		clientRepo := memory.NewClientRepository()
		userRepo := memory.NewUserRepository()
		authCodeRepo := memory.NewAuthCodeRepository()
		refreshTokenRepo := memory.NewRefreshTokenRepository()
		if err := seedMemory(clientRepo, userRepo); err != nil {
			return nil, nil, nil, nil, noopClose, fmt.Errorf("seed demo data (memory): %w", err)
		}
		slog.Info("authz: persistence configured", "mode", mode)
		return clientRepo, userRepo, authCodeRepo, refreshTokenRepo, noopClose, nil

	case postgres.ModePostgres:
		writerDB, readerDB, closeFn, err := postgres.OpenPair(ctx, writerCfg, readerCfg)
		if err != nil {
			return nil, nil, nil, nil, noopClose, fmt.Errorf("postgres open pair: %w", err)
		}
		// Seed writes go through the writer pool (seedPostgres is an
		// idempotent upsert -- see its doc comment -- so it must never
		// run against a lagging replica).
		if err := seedPostgres(ctx, writerDB); err != nil {
			_ = closeFn()
			return nil, nil, nil, nil, noopClose, fmt.Errorf("seed demo data (postgres): %w", err)
		}
		// Reader pool: seeded, read-only-at-runtime aggregates.
		clientRepo := postgres.NewClientRepository(readerDB)
		userRepo := postgres.NewUserRepository(readerDB)
		// Writer pool: single-use token aggregates, pinned for both
		// reads and writes (see func doc comment).
		authCodeRepo := postgres.NewAuthCodeRepository(writerDB)
		refreshTokenRepo := postgres.NewRefreshTokenRepository(writerDB)
		slog.Info("authz: persistence configured", "mode", mode)
		return clientRepo, userRepo, authCodeRepo, refreshTokenRepo, closeFn, nil

	default:
		// Unreachable: SelectMode only ever returns ModeMemory,
		// ModePostgres, or a non-nil error.
		return nil, nil, nil, nil, noopClose, fmt.Errorf("select persistence mode: unexpected mode %q", mode)
	}
}

// buildDemoClient constructs this authorization server's single demo
// OAuth client (see the demoClientID/demoRedirectURI package
// constants). It is shared by both persistence backends' seed paths
// (seedMemory / seedPostgres) so the demo data itself is defined
// exactly once.
func buildDemoClient() (*client.Client, error) {
	clientID, err := client.ParseClientID(demoClientID)
	if err != nil {
		return nil, fmt.Errorf("build demo client: %w", err)
	}
	redirectURI, err := client.NewRedirectURI(demoRedirectURI)
	if err != nil {
		return nil, fmt.Errorf("build demo client: %w", err)
	}
	return client.New(
		clientID,
		[]client.RedirectURI{redirectURI},
		[]string{"openid", "profile", "email"},
		[]string{"code"},
		[]string{"authorization_code", "refresh_token"},
	), nil
}

// buildDemoUser constructs this authorization server's single demo
// user. The demo user's password is generated fresh on every call
// rather than hardcoded, even though this sample's wiring never
// checks it (see service.AuthorizationService.resolveOwner: there is
// no login UI, so User.VerifyPassword is never called in the current
// request flow). It exists so the aggregate's shape matches a real
// IdP and can be wired to an actual login handler later. Shared by
// both persistence backends' seed paths.
func buildDemoUser() (*user.User, error) {
	userID, err := user.ParseUserID(demoUserID)
	if err != nil {
		return nil, fmt.Errorf("build demo user: %w", err)
	}
	username, err := user.NewUsername(demoUsername)
	if err != nil {
		return nil, fmt.Errorf("build demo user: %w", err)
	}
	profile, err := user.NewProfile(demoUserName, demoUserEmail)
	if err != nil {
		return nil, fmt.Errorf("build demo user: %w", err)
	}
	password, err := randomSecret(32)
	if err != nil {
		return nil, fmt.Errorf("build demo user: %w", err)
	}
	return user.New(userID, username, password, profile), nil
}

// seedMemory registers this sample authorization server's demo client
// and demo user into in-memory repositories. It runs once, at
// startup; there is no admin API to register additional
// clients/users (out of scope for this DDD layering sample -- see
// README.md). This is the original (pre-SPEC-005) seed behavior,
// preserved unchanged for the in-memory persistence path.
func seedMemory(clientRepo *memory.ClientRepository, userRepo *memory.UserRepository) error {
	demoClient, err := buildDemoClient()
	if err != nil {
		return fmt.Errorf("seed client: %w", err)
	}
	clientRepo.Seed(demoClient)

	demoUser, err := buildDemoUser()
	if err != nil {
		return fmt.Errorf("seed user: %w", err)
	}
	userRepo.Seed(demoUser)

	return nil
}

// seedPostgres idempotently upserts the same demo client/user data
// seedMemory registers, via postgres.SeedClient/SeedUser
// (docs/plans/SPEC-005-plan.md §1.2 "seed": a startup idempotent
// upsert -- not a migration-embedded seed, so the demo user's
// freshly-generated password is never committed to a SQL file, and
// repeated process starts converge on the same demo row instead of
// erroring on the second run).
func seedPostgres(ctx context.Context, db *sql.DB) error {
	demoClient, err := buildDemoClient()
	if err != nil {
		return fmt.Errorf("seed client: %w", err)
	}
	if err := postgres.SeedClient(ctx, db, demoClient); err != nil {
		return fmt.Errorf("seed client: %w", err)
	}

	demoUser, err := buildDemoUser()
	if err != nil {
		return fmt.Errorf("seed user: %w", err)
	}
	if err := postgres.SeedUser(ctx, db, demoUser); err != nil {
		return fmt.Errorf("seed user: %w", err)
	}

	return nil
}

// randomSecret generates a cryptographically random, base64url-encoded
// secret carrying n bytes of entropy.
func randomSecret(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("authz: generate random secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
