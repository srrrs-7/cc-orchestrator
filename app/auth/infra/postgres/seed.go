package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// clientSecretHashParam converts the client's optional secret hash
// into a sql.NullString for the UpsertClient query: nil (public
// client) maps to {Valid: false}; non-nil (confidential client) maps
// to {String: *h, Valid: true}.
func clientSecretHashParam(c *client.Client) sql.NullString {
	if h := c.SecretHash(); h != nil {
		return sql.NullString{String: *h, Valid: true}
	}
	return sql.NullString{}
}

// SeedClient idempotently inserts (or overwrites, keyed by ID) c into
// the clients table via an upsert (schema/queries/clients.sql's
// UpsertClient: INSERT ... ON CONFLICT (id) DO UPDATE), so calling it
// again with the same demo data (e.g. on every process start) MUST
// converge on the same row rather than erroring on the second run.
//
// client.Repository itself is read-only (FindByID only; see
// domain/client/repository.go), so this seed path necessarily lives
// outside that interface. Production wiring is cmd/authz/main.go's
// persistence block, which calls this once at startup (SPEC-011:
// Postgres is the sole persistence backend).
func SeedClient(ctx context.Context, db *sql.DB, c *client.Client) error {
	redirectURIs := make([]string, 0, len(c.RedirectURIs()))
	for _, uri := range c.RedirectURIs() {
		redirectURIs = append(redirectURIs, uri.String())
	}

	redirectURIsJSON, err := encodeStringSlice(redirectURIs)
	if err != nil {
		return fmt.Errorf("postgres: seed client: encode redirect_uris: %w", err)
	}
	allowedScopesJSON, err := encodeStringSlice(c.AllowedScopes())
	if err != nil {
		return fmt.Errorf("postgres: seed client: encode allowed_scopes: %w", err)
	}
	responseTypesJSON, err := encodeStringSlice(c.ResponseTypes())
	if err != nil {
		return fmt.Errorf("postgres: seed client: encode response_types: %w", err)
	}
	grantTypesJSON, err := encodeStringSlice(c.GrantTypes())
	if err != nil {
		return fmt.Errorf("postgres: seed client: encode grant_types: %w", err)
	}

	q := sqlcgen.New(db)
	if err := q.UpsertClient(ctx, sqlcgen.UpsertClientParams{
		ID:               c.ID().String(),
		RedirectUris:     redirectURIsJSON,
		AllowedScopes:    allowedScopesJSON,
		ResponseTypes:    responseTypesJSON,
		GrantTypes:       grantTypesJSON,
		ClientSecretHash: clientSecretHashParam(c),
	}); err != nil {
		return fmt.Errorf("postgres: seed client: %w", err)
	}
	return nil
}

// SeedUser idempotently inserts (or overwrites, keyed by ID) u into
// the users table via an upsert (schema/queries/users.sql's UpsertUser:
// INSERT ... ON CONFLICT (id) DO UPDATE); see SeedClient's doc
// comment for the rationale (user.Repository is also read-only).
//
// u.PasswordHash() is written as the bcrypt hash produced by
// domain/user.New at startup (cmd/authz/main.go's buildDemoUser).
func SeedUser(ctx context.Context, db *sql.DB, u *user.User) error {
	q := sqlcgen.New(db)
	if err := q.UpsertUser(ctx, sqlcgen.UpsertUserParams{
		ID:           u.ID().String(),
		Username:     u.Username().String(),
		PasswordHash: u.PasswordHash(),
		ProfileName:  u.Profile().Name(),
		ProfileEmail: u.Profile().Email(),
	}); err != nil {
		return fmt.Errorf("postgres: seed user: %w", err)
	}
	return nil
}
