package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/consent"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// ConsentRepository is a Postgres-backed consent.Repository (ISSUE-032).
type ConsentRepository struct {
	q *sqlcgen.Queries
}

var _ consent.Repository = (*ConsentRepository)(nil)

// NewConsentRepository builds a ConsentRepository backed by db.
func NewConsentRepository(db *sql.DB) *ConsentRepository {
	return &ConsentRepository{q: sqlcgen.New(db)}
}

// HasGrant reports whether a matching consent row exists.
func (r *ConsentRepository) HasGrant(ctx context.Context, userID, clientID, scope string) (bool, error) {
	has, err := r.q.HasConsent(ctx, sqlcgen.HasConsentParams{
		UserID:   userID,
		ClientID: clientID,
		Scope:    scope,
	})
	if err != nil {
		return false, fmt.Errorf("postgres: has consent: %w", err)
	}
	return has, nil
}

// SaveGrant upserts a consent grant.
func (r *ConsentRepository) SaveGrant(ctx context.Context, userID, clientID, scope string) error {
	if err := r.q.UpsertConsent(ctx, sqlcgen.UpsertConsentParams{
		UserID:   userID,
		ClientID: clientID,
		Scope:    scope,
	}); err != nil {
		return fmt.Errorf("postgres: save consent: %w", err)
	}
	return nil
}
