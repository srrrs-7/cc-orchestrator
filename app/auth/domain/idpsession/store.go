// Package idpsession models the authorization server's own login session
// (distinct from OAuth tokens issued to relying parties).
package idpsession

import (
	"context"
	"errors"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// ErrNotFound is returned when a session or pending authorize record
// does not exist or has expired.
var ErrNotFound = errors.New("idpsession: not found")

// Session represents an authenticated resource owner at the IdP.
type Session struct {
	ID        string
	UserID    user.UserID
	ExpiresAt time.Time
}

// PendingAuthorize holds an in-flight /authorize query until login completes.
type PendingAuthorize struct {
	ID        string
	RawQuery  string
	ExpiresAt time.Time
}

// Store persists IdP sessions and pending authorize requests.
type Store interface {
	CreateSession(ctx context.Context, userID user.UserID, ttl time.Duration) (Session, error)
	FindSession(ctx context.Context, id string) (Session, error)
	DeleteSession(ctx context.Context, id string) error

	SavePendingAuthorize(ctx context.Context, rawQuery string, ttl time.Duration) (PendingAuthorize, error)
	FindPendingAuthorize(ctx context.Context, id string) (PendingAuthorize, error)
	ConsumePendingAuthorize(ctx context.Context, id string) (PendingAuthorize, error)
}
