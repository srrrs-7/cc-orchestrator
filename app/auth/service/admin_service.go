package service

import (
	"context"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// AdminService handles administrative operations for client and user
// management (ISSUE-039). It is a thin application-layer coordinator
// between the admin HTTP handler (route/admin_handler.go) and the
// write-side persistence ports (client.Writer / user.Writer).
//
// AdminService is intentionally separate from AuthorizationService so
// the composition root can wire write-pool connections only here,
// leaving the read-only authorization path on the reader pool.
type AdminService struct {
	clients client.Writer
	users   user.Writer
}

// NewAdminService constructs an AdminService backed by the given
// write-side persistence ports.
func NewAdminService(clients client.Writer, users user.Writer) *AdminService {
	return &AdminService{clients: clients, users: users}
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
