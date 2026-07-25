package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// ClientWriter is a Postgres-backed implementation of client.Writer
// (ISSUE-039). It uses the same UpsertClient SQL query as SeedClient
// (seed.go) for idempotent client registration.
//
// The composition root wires ClientWriter to the writer pool so that
// admin-API registrations reach the primary, not a read replica.
type ClientWriter struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// var _ client.Writer = (*ClientWriter)(nil) verifies at compile time
// that ClientWriter satisfies the domain's Writer interface.
var _ client.Writer = (*ClientWriter)(nil)

// NewClientWriter builds a ClientWriter backed by db.
func NewClientWriter(db *sql.DB) *ClientWriter {
	return &ClientWriter{db: db, q: sqlcgen.New(db)}
}

// Save upserts c into the clients table (INSERT ... ON CONFLICT (id)
// DO UPDATE). Calling Save multiple times with the same ClientID
// converges idempotently on the latest state.
func (w *ClientWriter) Save(ctx context.Context, c *client.Client) error {
	redirectURIs := make([]string, 0, len(c.RedirectURIs()))
	for _, uri := range c.RedirectURIs() {
		redirectURIs = append(redirectURIs, uri.String())
	}

	redirectURIsJSON, err := encodeStringSlice(redirectURIs)
	if err != nil {
		return fmt.Errorf("postgres: client writer: encode redirect_uris: %w", err)
	}
	allowedScopesJSON, err := encodeStringSlice(c.AllowedScopes())
	if err != nil {
		return fmt.Errorf("postgres: client writer: encode allowed_scopes: %w", err)
	}
	responseTypesJSON, err := encodeStringSlice(c.ResponseTypes())
	if err != nil {
		return fmt.Errorf("postgres: client writer: encode response_types: %w", err)
	}
	grantTypesJSON, err := encodeStringSlice(c.GrantTypes())
	if err != nil {
		return fmt.Errorf("postgres: client writer: encode grant_types: %w", err)
	}

	if err := w.q.UpsertClient(ctx, sqlcgen.UpsertClientParams{
		ID:               c.ID().String(),
		RedirectUris:     redirectURIsJSON,
		AllowedScopes:    allowedScopesJSON,
		ResponseTypes:    responseTypesJSON,
		GrantTypes:       grantTypesJSON,
		ClientSecretHash: clientSecretHashParam(c),
	}); err != nil {
		return fmt.Errorf("postgres: client writer: save: %w", err)
	}
	return nil
}

// DeleteClient removes the client and dependent rows in one transaction.
func (w *ClientWriter) DeleteClient(ctx context.Context, id client.ClientID) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: client writer: delete client: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := w.q.WithTx(tx)
	clientID := id.String()

	if err := qtx.DeleteConsentsByClientID(ctx, clientID); err != nil {
		return fmt.Errorf("postgres: client writer: delete consents: %w", err)
	}
	if err := qtx.DeleteRefreshTokensByClientID(ctx, clientID); err != nil {
		return fmt.Errorf("postgres: client writer: delete refresh tokens: %w", err)
	}
	if err := qtx.DeleteAuthCodesByClientID(ctx, clientID); err != nil {
		return fmt.Errorf("postgres: client writer: delete auth codes: %w", err)
	}

	rows, err := qtx.DeleteClient(ctx, clientID)
	if err != nil {
		return fmt.Errorf("postgres: client writer: delete client: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: client writer: delete client: %w", client.ErrNotFound)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgres: client writer: delete client: commit: %w", err)
	}
	return nil
}
