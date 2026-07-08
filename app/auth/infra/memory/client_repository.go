// Package memory provides in-memory infrastructure-layer
// implementations of the domain repository interfaces (client.Repository,
// user.Repository, authcode.Repository), useful for running the
// authorization server without an external datastore.
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
)

// ClientRepository is an in-memory, concurrency-safe implementation
// of client.Repository.
type ClientRepository struct {
	mu      sync.RWMutex
	clients map[client.ClientID]*client.Client
}

// var _ client.Repository = (*ClientRepository)(nil) verifies at
// compile time that ClientRepository satisfies the domain's
// Repository interface.
var _ client.Repository = (*ClientRepository)(nil)

// NewClientRepository builds an empty ClientRepository.
func NewClientRepository() *ClientRepository {
	return &ClientRepository{
		clients: make(map[client.ClientID]*client.Client),
	}
}

// Seed registers c in the repository. It is intended for use at
// startup (see cmd/authz/main.go) to pre-populate demo clients; it is
// otherwise identical to a save.
func (r *ClientRepository) Seed(c *client.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[c.ID()] = cloneClient(c)
}

// FindByID returns the Client with the given id, or
// client.ErrNotFound if none exists.
func (r *ClientRepository) FindByID(ctx context.Context, id client.ClientID) (*client.Client, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("memory: find client by id: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.clients[id]
	if !ok {
		return nil, fmt.Errorf("memory: find client by id: %w", client.ErrNotFound)
	}
	return cloneClient(c), nil
}

// cloneClient returns a new *client.Client built from c's state via
// client.Reconstruct, preventing the repository's stored data and the
// caller's data from aliasing (sharing) the same mutable object.
func cloneClient(c *client.Client) *client.Client {
	return client.Reconstruct(c.ID(), c.RedirectURIs(), c.AllowedScopes(), c.ResponseTypes(), c.GrantTypes())
}
