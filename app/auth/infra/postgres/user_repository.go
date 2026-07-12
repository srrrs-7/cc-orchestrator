package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// UserRepository is a Postgres-backed implementation of
// user.Repository (SPEC-005 R2). user.Repository is read-only
// (FindByID / FindByUsername only); population happens through
// SeedUser, called once at startup by cmd/authz/main.go's persistence
// wiring (SPEC-011: Postgres is the sole persistence backend).
type UserRepository struct {
	q *sqlcgen.Queries
}

// var _ user.Repository = (*UserRepository)(nil) verifies at compile
// time that UserRepository satisfies the domain's Repository
// interface.
var _ user.Repository = (*UserRepository)(nil)

// NewUserRepository builds a UserRepository backed by db.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{q: sqlcgen.New(db)}
}

// FindByID returns the User with the given id, or user.ErrNotFound if
// none exists. sql.ErrNoRows from the underlying query is never
// returned as-is: it is translated to the domain's sentinel error, per
// domain/user/repository.go's contract.
func (r *UserRepository) FindByID(ctx context.Context, id user.UserID) (*user.User, error) {
	row, err := r.q.GetUserByID(ctx, id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: find user by id: %w", user.ErrNotFound)
		}
		return nil, fmt.Errorf("postgres: find user by id: %w", err)
	}
	u, err := rowToUser(row)
	if err != nil {
		return nil, fmt.Errorf("postgres: find user by id: %w", err)
	}
	return u, nil
}

// FindByUsername returns the User with the given username, or
// user.ErrNotFound if none exists. The comparison is an exact,
// case-sensitive SQL "=" match (the users.username column carries no
// case-folding collation), matching infra/memory.UserRepository's
// exact `==` comparison.
func (r *UserRepository) FindByUsername(ctx context.Context, username user.Username) (*user.User, error) {
	row, err := r.q.GetUserByUsername(ctx, username.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("postgres: find user by username: %w", user.ErrNotFound)
		}
		return nil, fmt.Errorf("postgres: find user by username: %w", err)
	}
	u, err := rowToUser(row)
	if err != nil {
		return nil, fmt.Errorf("postgres: find user by username: %w", err)
	}
	return u, nil
}

// ListAll returns every User ordered by id. An empty table yields a
// nil slice, not an error.
func (r *UserRepository) ListAll(ctx context.Context) ([]*user.User, error) {
	rows, err := r.q.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	users := make([]*user.User, 0, len(rows))
	for _, row := range rows {
		u, err := rowToUser(row)
		if err != nil {
			return nil, fmt.Errorf("postgres: list users: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

// rowToUser reconstructs a *user.User from a persisted row.
func rowToUser(row sqlcgen.User) (*user.User, error) {
	id, err := user.ParseUserID(row.ID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}
	username, err := user.NewUsername(row.Username)
	if err != nil {
		return nil, fmt.Errorf("new username: %w", err)
	}
	profile, err := user.NewProfile(row.ProfileName, row.ProfileEmail)
	if err != nil {
		return nil, fmt.Errorf("new profile: %w", err)
	}
	return user.Reconstruct(id, username, row.PasswordHash, profile), nil
}
