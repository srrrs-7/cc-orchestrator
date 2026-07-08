package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// UserRepository is an in-memory, concurrency-safe implementation of
// user.Repository.
type UserRepository struct {
	mu   sync.RWMutex
	byID map[user.UserID]*user.User
}

// var _ user.Repository = (*UserRepository)(nil) verifies at compile
// time that UserRepository satisfies the domain's Repository
// interface.
var _ user.Repository = (*UserRepository)(nil)

// NewUserRepository builds an empty UserRepository.
func NewUserRepository() *UserRepository {
	return &UserRepository{
		byID: make(map[user.UserID]*user.User),
	}
}

// Seed registers u in the repository. It is intended for use at
// startup (see cmd/authz/main.go) to pre-populate demo users; it is
// otherwise identical to a save.
func (r *UserRepository) Seed(u *user.User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byID[u.ID()] = cloneUser(u)
}

// FindByID returns the User with the given id, or user.ErrNotFound if
// none exists.
func (r *UserRepository) FindByID(ctx context.Context, id user.UserID) (*user.User, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find user by id: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("memory: find user by id: %w", user.ErrNotFound)
	}
	return cloneUser(u), nil
}

// FindByUsername returns the User with the given username, or
// user.ErrNotFound if none exists.
func (r *UserRepository) FindByUsername(ctx context.Context, username user.Username) (*user.User, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find user by username: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.byID {
		if u.Username() == username {
			return cloneUser(u), nil
		}
	}
	return nil, fmt.Errorf("memory: find user by username: %w", user.ErrNotFound)
}

// cloneUser returns a new *user.User built from u's state via
// user.Reconstruct, preventing the repository's stored data and the
// caller's data from aliasing (sharing) the same mutable object.
func cloneUser(u *user.User) *user.User {
	return user.Reconstruct(u.ID(), u.Username(), u.Password(), u.Profile())
}
