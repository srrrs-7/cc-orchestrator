package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// SeedClient idempotently inserts (or overwrites, keyed by ID) c into
// the clients table via an upsert (db/queries/clients.sql's
// UpsertClient: INSERT ... ON CONFLICT (id) DO UPDATE), so calling it
// again with the same demo data (e.g. on every process start) MUST
// converge on the same row rather than erroring on the second run.
//
// client.Repository itself is read-only (FindByID only; see
// domain/client/repository.go), so this seed path necessarily lives
// outside that interface. Production wiring is cmd/authz/main.go's
// persistence block, which calls this once at startup when Postgres
// mode is selected (see SelectMode) -- mirroring what
// infra/memory.ClientRepository.Seed does for the in-memory path.
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
		ID:            c.ID().String(),
		RedirectUris:  redirectURIsJSON,
		AllowedScopes: allowedScopesJSON,
		ResponseTypes: responseTypesJSON,
		GrantTypes:    grantTypesJSON,
	}); err != nil {
		return fmt.Errorf("postgres: seed client: %w", err)
	}
	return nil
}

// SeedUser idempotently inserts (or overwrites, keyed by ID) u into
// the users table via an upsert (db/queries/users.sql's UpsertUser:
// INSERT ... ON CONFLICT (id) DO UPDATE); see SeedClient's doc
// comment for the rationale (user.Repository is also read-only).
//
// u.Password() is written as-is (plaintext): this mirrors the
// existing domain/user.User design and is a deliberate, documented
// scope limit of this Spec (docs/plans/SPEC-005-plan.md §6.1 R-b) --
// this function itself never receives or embeds a hardcoded password;
// the demo password remains generated at process startup by
// cmd/authz/main.go's seed(), same as the memory path.
func SeedUser(ctx context.Context, db *sql.DB, u *user.User) error {
	q := sqlcgen.New(db)
	if err := q.UpsertUser(ctx, sqlcgen.UpsertUserParams{
		ID:           u.ID().String(),
		Username:     u.Username().String(),
		Password:     u.Password(),
		ProfileName:  u.Profile().Name(),
		ProfileEmail: u.Profile().Email(),
	}); err != nil {
		return fmt.Errorf("postgres: seed user: %w", err)
	}
	return nil
}
