package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// AdminService handles administrative operations for client and user
// management (ISSUE-039). It is a thin application-layer coordinator
// between the admin HTTP handler (route/admin_handler.go) and the
// write-side persistence ports (client.Writer / user.Writer) plus the
// read-side ports used for listing (client.Repository / user.Repository).
//
// AdminService is intentionally separate from AuthorizationService so
// the composition root can wire write-pool connections only for
// mutations, leaving the read-only authorization path on the reader pool.
type AdminService struct {
	clients      client.Writer
	users        user.Writer
	clientReader client.Repository
	userReader   user.Repository
}

// NewAdminService constructs an AdminService backed by the given
// persistence ports.
func NewAdminService(
	clients client.Writer,
	users user.Writer,
	clientReader client.Repository,
	userReader user.Repository,
) *AdminService {
	return &AdminService{
		clients:      clients,
		users:        users,
		clientReader: clientReader,
		userReader:   userReader,
	}
}

// CreateClient persists c via the client.Writer port. The caller is
// responsible for constructing a valid *client.Client (public or
// confidential) before calling this method; AdminService itself adds
// no validation beyond what the domain constructors already enforce.
func (s *AdminService) CreateClient(ctx context.Context, c *client.Client) error {
	if err := s.clients.Save(ctx, c); err != nil {
		return fmt.Errorf("admin: create client: %w", err)
	}
	return nil
}

// CreateUser persists u via the user.Writer port. The caller must
// build a valid *user.User (with a bcrypt-hashed password via
// user.New) before calling this method.
func (s *AdminService) CreateUser(ctx context.Context, u *user.User) error {
	if err := s.users.CreateUser(ctx, u); err != nil {
		return fmt.Errorf("admin: create user: %w", err)
	}
	return nil
}

// ListClients returns every registered OAuth client ordered by id.
func (s *AdminService) ListClients(ctx context.Context) ([]*client.Client, error) {
	clients, err := s.clientReader.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("admin: list clients: %w", err)
	}
	return clients, nil
}

// ListUsers returns every registered resource owner ordered by id.
func (s *AdminService) ListUsers(ctx context.Context) ([]*user.User, error) {
	users, err := s.userReader.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("admin: list users: %w", err)
	}
	return users, nil
}

// GetUser returns a single user by id.
func (s *AdminService) GetUser(ctx context.Context, id user.UserID) (*user.User, error) {
	u, err := s.userReader.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("admin: get user: %w", err)
	}
	return u, nil
}

// DeleteUser removes a user and dependent authorization artifacts.
func (s *AdminService) DeleteUser(ctx context.Context, id user.UserID) error {
	if err := s.users.DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("admin: delete user: %w", err)
	}
	return nil
}

// GetClient returns a single OAuth client by id.
func (s *AdminService) GetClient(ctx context.Context, id client.ClientID) (*client.Client, error) {
	c, err := s.clientReader.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("admin: get client: %w", err)
	}
	return c, nil
}

// DeleteClient removes a client and dependent authorization artifacts.
func (s *AdminService) DeleteClient(ctx context.Context, id client.ClientID) error {
	if err := s.clients.DeleteClient(ctx, id); err != nil {
		return fmt.Errorf("admin: delete client: %w", err)
	}
	return nil
}

// IsNotFound reports whether err wraps a domain not-found sentinel.
func IsNotFound(err error) bool {
	return errors.Is(err, user.ErrNotFound) || errors.Is(err, client.ErrNotFound)
}
