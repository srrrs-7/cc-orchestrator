// Command authz is the composition root of the OAuth 2.0 authorization
// server / OpenID Provider sample application. It only wires
// dependencies together and manages the HTTP server's lifecycle; it
// holds no business logic itself.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/jwt"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/memory"
	"github.com/srrrs-7/cc-orchestrator/app/auth/route"
	"github.com/srrrs-7/cc-orchestrator/app/auth/service"
)

const (
	defaultPort     = "8080"
	defaultIssuer   = "http://localhost:8080"
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
	// fresh at process startup instead (see run() / seed()).
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

	issuer := os.Getenv("ISSUER")
	if issuer == "" {
		issuer = defaultIssuer
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

	// Repositories + demo seed data.
	clientRepo := memory.NewClientRepository()
	userRepo := memory.NewUserRepository()
	authCodeRepo := memory.NewAuthCodeRepository()

	if err := seed(clientRepo, userRepo); err != nil {
		return fmt.Errorf("authz: seed demo data: %w", err)
	}

	defaultUsername, err := user.NewUsername(demoUsername)
	if err != nil {
		return fmt.Errorf("authz: build default username: %w", err)
	}

	// Wiring: infra -> application service -> presentation.
	authSvc := service.NewAuthorizationService(clientRepo, userRepo, authCodeRepo, signer, issuer, defaultUsername)
	userInfoSvc := service.NewUserInfoService(userRepo, verifier, issuer)
	discoverySvc := service.NewDiscoveryService(issuer, keyProvider)
	handler := route.NewRouter(authSvc, userInfoSvc, discoverySvc)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("authz: server starting", "addr", srv.Addr, "issuer", issuer, "kid", kid)
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

// seed registers this sample authorization server's demo client and
// demo user. It runs once, at startup; there is no admin API to
// register additional clients/users (out of scope for this DDD
// layering sample -- see README.md).
func seed(clientRepo *memory.ClientRepository, userRepo *memory.UserRepository) error {
	clientID, err := client.ParseClientID(demoClientID)
	if err != nil {
		return fmt.Errorf("authz: seed client: %w", err)
	}
	redirectURI, err := client.NewRedirectURI(demoRedirectURI)
	if err != nil {
		return fmt.Errorf("authz: seed client: %w", err)
	}
	demoClient := client.New(
		clientID,
		[]client.RedirectURI{redirectURI},
		[]string{"openid", "profile", "email"},
		[]string{"code"},
		[]string{"authorization_code"},
	)
	clientRepo.Seed(demoClient)

	userID, err := user.ParseUserID(demoUserID)
	if err != nil {
		return fmt.Errorf("authz: seed user: %w", err)
	}
	username, err := user.NewUsername(demoUsername)
	if err != nil {
		return fmt.Errorf("authz: seed user: %w", err)
	}
	profile, err := user.NewProfile(demoUserName, demoUserEmail)
	if err != nil {
		return fmt.Errorf("authz: seed user: %w", err)
	}
	// The demo user's password is generated fresh at startup rather
	// than hardcoded, even though this sample's wiring never checks it
	// (see service.AuthorizationService.resolveOwner: there is no
	// login UI, so User.VerifyPassword is never called in the current
	// request flow). It exists so the aggregate's shape matches a real
	// IdP and can be wired to an actual login handler later.
	password, err := randomSecret(32)
	if err != nil {
		return fmt.Errorf("authz: seed user: %w", err)
	}
	demoUser := user.New(userID, username, password, profile)
	userRepo.Seed(demoUser)

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
