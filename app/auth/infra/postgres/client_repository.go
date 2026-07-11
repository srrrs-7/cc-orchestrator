package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// ClientRepository is a Postgres-backed implementation of
// client.Repository (SPEC-005 R2). client.Repository is read-only
// (FindByID only); population happens through SeedClient, called once
// at startup by cmd/authz/main.go's persistence wiring (SPEC-011:
// Postgres is the sole persistence backend).
type ClientRepository struct {
	q *sqlcgen.Queries
}

// var _ client.Repository = (*ClientRepository)(nil) verifies at
// compile time that ClientRepository satisfies the domain's
// Repository interface.
var _ client.Repository = (*ClientRepository)(nil)

// NewClientRepository builds a ClientRepository backed by db.
func NewClientRepository(db *sql.DB) *ClientRepository {
	return &ClientRepository{q: sqlcgen.New(db)}
}

// FindByID returns the Client with the given id, or client.ErrNotFound
// if none exists. sql.ErrNoRows from the underlying query is never
// returned as-is: it is translated to the domain's sentinel error, per
// domain/client/repository.go's contract.
func (r *ClientRepository) FindByID(ctx context.Context, id client.ClientID) (*client.Client, error) {
	row, err := r.q.GetClientByID(ctx, id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: find client by id: %w", client.ErrNotFound)
		}
		return nil, fmt.Errorf("postgres: find client by id: %w", err)
	}
	c, err := rowToClient(row)
	if err != nil {
		return nil, fmt.Errorf("postgres: find client by id: %w", err)
	}
	return c, nil
}

// rowToClient reconstructs a *client.Client from a persisted row,
// decoding the four jsonb-encoded multi-valued attributes
// (redirect_uris / allowed_scopes / response_types / grant_types) back
// into []string via encoding/json (plan §1.2: jsonb chosen over
// text[] for stable database/sql + pgx stdlib scanning; the decode
// side lives here rather than in sqlcgen since sqlc's database/sql
// backend has no jsonb-to-typed-slice mapping of its own).
func rowToClient(row sqlcgen.Client) (*client.Client, error) {
	id, err := client.ParseClientID(row.ID)
	if err != nil {
		return nil, fmt.Errorf("parse client id: %w", err)
	}

	redirectURIStrings, err := decodeStringSlice(row.RedirectUris)
	if err != nil {
		return nil, fmt.Errorf("decode redirect_uris: %w", err)
	}
	redirectURIs := make([]client.RedirectURI, 0, len(redirectURIStrings))
	for _, s := range redirectURIStrings {
		uri, err := client.NewRedirectURI(s)
		if err != nil {
			return nil, fmt.Errorf("decode redirect_uris: %w", err)
		}
		redirectURIs = append(redirectURIs, uri)
	}

	allowedScopes, err := decodeStringSlice(row.AllowedScopes)
	if err != nil {
		return nil, fmt.Errorf("decode allowed_scopes: %w", err)
	}
	responseTypes, err := decodeStringSlice(row.ResponseTypes)
	if err != nil {
		return nil, fmt.Errorf("decode response_types: %w", err)
	}
	grantTypes, err := decodeStringSlice(row.GrantTypes)
	if err != nil {
		return nil, fmt.Errorf("decode grant_types: %w", err)
	}

	return client.Reconstruct(id, redirectURIs, allowedScopes, responseTypes, grantTypes), nil
}

// decodeStringSlice unmarshals a jsonb column (stored as a JSON array
// of strings) into a []string. An empty/absent value decodes to a nil
// slice; client.New/client.Reconstruct treat a nil slice the same as
// an empty one (see client.go's toSet), so this is not lossy.
func decodeStringSlice(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

// encodeStringSlice marshals values (nil-safe: a nil slice encodes as
// "[]", never SQL NULL, matching the clients table's jsonb NOT NULL
// columns) into a jsonb-ready json.RawMessage. Used by SeedClient
// (seed.go).
func encodeStringSlice(values []string) (json.RawMessage, error) {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
