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
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/consent"
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
	demoClientID           = "demo-client"
	demoRedirectURI        = "http://localhost:3000/callback"
	demoRedirectURICompose = "http://localhost:8080/callback"
	demoUsername           = "demo-user"
	demoUserID             = "demo-user-id"
	demoUserName           = "Demo User"
	demoUserEmail          = "demo-user@example.com"
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
	if err := e.validate(); err != nil {
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

	// Repositories + demo seed data (SPEC-011: Postgres is the sole
	// persistence backend). SPEC-010: setupPersistence opens a
	// writer/reader pool pair via postgres.OpenPair and wires each
	// repository to the pool the "auth の correctness-critical read の
	// 配置" table (docs/plans/SPEC-010-plan.md) assigns it.
	clientRepo, userRepo, authCodeRepo, refreshTokenRepo, consentRepo, closePersistence, demoPassword, err := setupPersistence(ctx, e.writerConfig(), e.readerConfig(), e.DemoPassword)
	if err != nil {
		return fmt.Errorf("authz: setup persistence: %w", err)
	}
	defer func() {
		if err := closePersistence(); err != nil {
			slog.Error("authz: close persistence", "error", err)
		}
	}()
	if e.DemoPassword != "" || os.Getenv("DEMO_LOG_PASSWORD") == "1" {
		slog.Info("authz: demo user password", "username", demoUsername, "password", demoPassword)
	}

	sessionStore := memory.NewIdPSessionStore()

	// Wiring: infra -> application service -> presentation.
	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, refreshTokenRepo, signer, e.Issuer)
	authnSvc := service.NewAuthenticationService(userRepo, sessionStore)
	consentSvc := service.NewConsentService(consentRepo)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, e.Issuer)
	discoverySvc := service.NewDiscoveryService(e.Issuer, keyProvider)
	handler := route.NewRouter(authSvc, authnSvc, consentSvc, userInfoSvc, discoverySvc, route.RouterConfig{
		Issuer:        e.Issuer,
		SecureCookies: route.SecureCookiesFromIssuer(e.Issuer),
	})

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

// setupPersistence is the persistence composition block (SPEC-011:
// Postgres is the sole backend). It opens a writer/reader pool pair
// via postgres.OpenPair and wires each repository according to
// docs/plans/SPEC-010-plan.md's "auth の correctness-critical read の
// 配置" table:
//
//   - client/user (seeded once at startup, never written at runtime)
//     → reader pool
//   - authcode/refreshtoken (single-use tokens whose read-after-write
//     correctness must not be exposed to replica lag) → writer pool
//     for both reads and writes
//
// writerCfg/readerCfg are Env.writerConfig()/Env.readerConfig()
// (SPEC-010): postgres.OpenPair opens a single shared *sql.DB when
// readerCfg == writerCfg (the DB_READER_* -unset default) or two
// independent pools otherwise.
//
// It returns the four repositories as their domain-declared interfaces
// plus a closePersistence func the caller MUST defer-call during
// shutdown to release pooled connections.
func setupPersistence(ctx context.Context, writerCfg, readerCfg postgres.Config, demoPassword string) (client.Repository, user.Repository, authcode.Repository, refreshtoken.Repository, consent.Repository, func() error, string, error) {
	noopClose := func() error { return nil }

	writerDB, readerDB, closeFn, err := postgres.OpenPair(ctx, writerCfg, readerCfg)
	if err != nil {
		return nil, nil, nil, nil, nil, noopClose, "", fmt.Errorf("postgres open pair: %w", err)
	}
	seededPassword, err := seedPostgres(ctx, writerDB, demoPassword)
	if err != nil {
		_ = closeFn()
		return nil, nil, nil, nil, nil, noopClose, "", fmt.Errorf("seed demo data (postgres): %w", err)
	}
	// Reader pool: seeded, read-only-at-runtime aggregates.
	clientRepo := postgres.NewClientRepository(readerDB)
	userRepo := postgres.NewUserRepository(readerDB)
	// Writer pool: single-use token aggregates, pinned for both
	// reads and writes (see func doc comment).
	authCodeRepo := postgres.NewAuthCodeRepository(writerDB)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(writerDB)
	consentRepo := postgres.NewConsentRepository(writerDB)
	slog.Info("authz: persistence configured", "mode", "postgres")
	return clientRepo, userRepo, authCodeRepo, refreshTokenRepo, consentRepo, closeFn, seededPassword, nil
}

// buildDemoClient constructs this authorization server's single demo
// OAuth client (see the demoClientID/demoRedirectURI package
// constants). It is shared by seedPostgres so the demo data itself is
// defined exactly once.
func buildDemoClient() (*client.Client, error) {
	clientID, err := client.ParseClientID(demoClientID)
	if err != nil {
		return nil, fmt.Errorf("build demo client: %w", err)
	}
	redirectURI, err := client.NewRedirectURI(demoRedirectURI)
	if err != nil {
		return nil, fmt.Errorf("build demo client: %w", err)
	}
	redirectURICompose, err := client.NewRedirectURI(demoRedirectURICompose)
	if err != nil {
		return nil, fmt.Errorf("build demo client: %w", err)
	}
	return client.New(
		clientID,
		[]client.RedirectURI{redirectURI, redirectURICompose},
		[]string{"openid", "profile", "email"},
		[]string{"code"},
		[]string{"authorization_code", "refresh_token"},
	), nil
}

// buildDemoUser constructs this authorization server's single demo user.
// When plaintextPassword is empty a random secret is generated.
func buildDemoUser(plaintextPassword string) (*user.User, string, error) {
	userID, err := user.ParseUserID(demoUserID)
	if err != nil {
		return nil, "", fmt.Errorf("build demo user: %w", err)
	}
	username, err := user.NewUsername(demoUsername)
	if err != nil {
		return nil, "", fmt.Errorf("build demo user: %w", err)
	}
	profile, err := user.NewProfile(demoUserName, demoUserEmail)
	if err != nil {
		return nil, "", fmt.Errorf("build demo user: %w", err)
	}
	password := plaintextPassword
	if password == "" {
		password, err = randomSecret(32)
		if err != nil {
			return nil, "", fmt.Errorf("build demo user: %w", err)
		}
	}
	u, err := user.New(userID, username, password, profile)
	if err != nil {
		return nil, "", fmt.Errorf("build demo user: %w", err)
	}
	return u, password, nil
}

// seedPostgres idempotently upserts the demo client/user data via
// postgres.SeedClient/SeedUser. It returns the plaintext demo password
// used for optional startup logging.
func seedPostgres(ctx context.Context, db *sql.DB, demoPassword string) (string, error) {
	demoClient, err := buildDemoClient()
	if err != nil {
		return "", fmt.Errorf("seed client: %w", err)
	}
	if err := postgres.SeedClient(ctx, db, demoClient); err != nil {
		return "", fmt.Errorf("seed client: %w", err)
	}

	demoUser, plaintext, err := buildDemoUser(demoPassword)
	if err != nil {
		return "", fmt.Errorf("seed user: %w", err)
	}
	if err := postgres.SeedUser(ctx, db, demoUser); err != nil {
		return "", fmt.Errorf("seed user: %w", err)
	}

	return plaintext, nil
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
