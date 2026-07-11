package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/idpsession"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// ErrInvalidCredentials is returned when username/password authentication fails.
var ErrInvalidCredentials = errors.New("service: invalid credentials")

const (
	defaultSessionTTL          = 24 * time.Hour
	defaultPendingAuthorizeTTL = 10 * time.Minute
)

// AuthenticationService handles IdP login sessions (resource owner
// authentication at the authorization server itself).
type AuthenticationService struct {
	users    user.Repository
	sessions idpsession.Store
}

// NewAuthenticationService builds an AuthenticationService.
func NewAuthenticationService(users user.Repository, sessions idpsession.Store) *AuthenticationService {
	return &AuthenticationService{users: users, sessions: sessions}
}

// Login verifies credentials and creates a new IdP session.
func (s *AuthenticationService) Login(ctx context.Context, usernameRaw, password string) (idpsession.Session, error) {
	username, err := user.NewUsername(usernameRaw)
	if err != nil {
		return idpsession.Session{}, ErrInvalidCredentials
	}
	u, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return idpsession.Session{}, ErrInvalidCredentials
		}
		return idpsession.Session{}, fmt.Errorf("service: login: %w", err)
	}
	if !u.VerifyPassword(password) {
		return idpsession.Session{}, ErrInvalidCredentials
	}
	sess, err := s.sessions.CreateSession(ctx, u.ID(), defaultSessionTTL)
	if err != nil {
		return idpsession.Session{}, fmt.Errorf("service: login: %w", err)
	}
	return sess, nil
}

// UserFromSession loads the resource owner for an IdP session id.
func (s *AuthenticationService) UserFromSession(ctx context.Context, sessionID string) (*user.User, error) {
	if sessionID == "" {
		return nil, idpsession.ErrNotFound
	}
	sess, err := s.sessions.FindSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, fmt.Errorf("service: user from session: %w", err)
	}
	return u, nil
}

// FindSession returns the IdP session for sessionID. It returns
// idpsession.ErrNotFound when sessionID is empty or the session does
// not exist / has expired. Use this to access session metadata (e.g.
// AuthenticatedAt for OIDC auth_time) without also resolving the user.
func (s *AuthenticationService) FindSession(ctx context.Context, sessionID string) (idpsession.Session, error) {
	if sessionID == "" {
		return idpsession.Session{}, idpsession.ErrNotFound
	}
	return s.sessions.FindSession(ctx, sessionID)
}

// SavePendingAuthorize stores an in-flight /authorize query until login completes.
func (s *AuthenticationService) SavePendingAuthorize(ctx context.Context, rawQuery string) (string, error) {
	p, err := s.sessions.SavePendingAuthorize(ctx, rawQuery, defaultPendingAuthorizeTTL)
	if err != nil {
		return "", fmt.Errorf("service: save pending authorize: %w", err)
	}
	return p.ID, nil
}

// Logout deletes the IdP session identified by sessionID.
func (s *AuthenticationService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.sessions.DeleteSession(ctx, sessionID)
}

// PeekPendingAuthorize returns a pending authorize query without deleting it.
func (s *AuthenticationService) PeekPendingAuthorize(ctx context.Context, id string) (string, error) {
	p, err := s.sessions.FindPendingAuthorize(ctx, id)
	if err != nil {
		return "", err
	}
	return p.RawQuery, nil
}

// ConsumePendingAuthorize loads and deletes a pending authorize record.
func (s *AuthenticationService) ConsumePendingAuthorize(ctx context.Context, id string) (string, error) {
	p, err := s.sessions.ConsumePendingAuthorize(ctx, id)
	if err != nil {
		return "", err
	}
	return p.RawQuery, nil
}
