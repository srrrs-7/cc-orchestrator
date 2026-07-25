package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// UserWriter is a Postgres-backed implementation of user.Writer
// (ISSUE-039). It uses the same UpsertUser SQL query as SeedUser
// (seed.go) for idempotent user registration.
//
// The composition root wires UserWriter to the writer pool so that
// admin-API registrations reach the primary, not a read replica.
type UserWriter struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// var _ user.Writer = (*UserWriter)(nil) verifies at compile time
// that UserWriter satisfies the domain's Writer interface.
var _ user.Writer = (*UserWriter)(nil)

// NewUserWriter builds a UserWriter backed by db.
func NewUserWriter(db *sql.DB) *UserWriter {
	return &UserWriter{db: db, q: sqlcgen.New(db)}
}

// CreateUser upserts u into the users table (INSERT ... ON CONFLICT
// (id) DO UPDATE). Calling CreateUser multiple times with the same
// UserID converges idempotently on the latest state.
func (w *UserWriter) CreateUser(ctx context.Context, u *user.User) error {
	if err := w.q.UpsertUser(ctx, sqlcgen.UpsertUserParams{
		ID:           u.ID().String(),
		Username:     u.Username().String(),
		PasswordHash: u.PasswordHash(),
		ProfileName:  u.Profile().Name(),
		ProfileEmail: u.Profile().Email(),
	}); err != nil {
		return fmt.Errorf("postgres: user writer: create user: %w", err)
	}
	return nil
}

// DeleteUser removes the user and dependent rows in one transaction.
func (w *UserWriter) DeleteUser(ctx context.Context, id user.UserID) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: user writer: delete user: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := w.q.WithTx(tx)
	userID := id.String()

	if err := qtx.DeleteConsentsByUserID(ctx, userID); err != nil {
		return fmt.Errorf("postgres: user writer: delete consents: %w", err)
	}
	if err := qtx.DeleteRefreshTokensByUserID(ctx, userID); err != nil {
		return fmt.Errorf("postgres: user writer: delete refresh tokens: %w", err)
	}
	if err := qtx.DeleteAuthCodesByUserID(ctx, userID); err != nil {
		return fmt.Errorf("postgres: user writer: delete auth codes: %w", err)
	}

	rows, err := qtx.DeleteUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("postgres: user writer: delete user: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("postgres: user writer: delete user: %w", user.ErrNotFound)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgres: user writer: delete user: commit: %w", err)
	}
	return nil
}
