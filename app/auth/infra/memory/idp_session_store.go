package memory

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/idpsession"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// IdPSessionStore is an in-memory idpsession.Store with TTL enforcement
// on read. It is suitable for the sample authorization server; a
// production deployment would persist sessions in Postgres or Redis.
type IdPSessionStore struct {
	mu       sync.Mutex
	sessions map[string]idpsession.Session
	pending  map[string]idpsession.PendingAuthorize
}

// NewIdPSessionStore returns an empty IdPSessionStore.
func NewIdPSessionStore() *IdPSessionStore {
	return &IdPSessionStore{
		sessions: make(map[string]idpsession.Session),
		pending:  make(map[string]idpsession.PendingAuthorize),
	}
}

var _ idpsession.Store = (*IdPSessionStore)(nil)

func (s *IdPSessionStore) CreateSession(ctx context.Context, userID user.UserID, ttl time.Duration) (idpsession.Session, error) {
	if err := ctx.Err(); err != nil {
		return idpsession.Session{}, err
	}
	id, err := randomID()
	if err != nil {
		return idpsession.Session{}, err
	}
	sess := idpsession.Session{
		ID:        id,
		UserID:    userID,
		ExpiresAt: time.Now().Add(ttl),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *IdPSessionStore) FindSession(ctx context.Context, id string) (idpsession.Session, error) {
	if err := ctx.Err(); err != nil {
		return idpsession.Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.ExpiresAt) {
		delete(s.sessions, id)
		return idpsession.Session{}, fmt.Errorf("memory idp session: %w", idpsession.ErrNotFound)
	}
	return sess, nil
}

func (s *IdPSessionStore) DeleteSession(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	return nil
}

func (s *IdPSessionStore) SavePendingAuthorize(ctx context.Context, rawQuery string, ttl time.Duration) (idpsession.PendingAuthorize, error) {
	if err := ctx.Err(); err != nil {
		return idpsession.PendingAuthorize{}, err
	}
	id, err := randomID()
	if err != nil {
		return idpsession.PendingAuthorize{}, err
	}
	p := idpsession.PendingAuthorize{
		ID:        id,
		RawQuery:  rawQuery,
		ExpiresAt: time.Now().Add(ttl),
	}
	s.mu.Lock()
	s.pending[id] = p
	s.mu.Unlock()
	return p, nil
}

func (s *IdPSessionStore) ConsumePendingAuthorize(ctx context.Context, id string) (idpsession.PendingAuthorize, error) {
	if err := ctx.Err(); err != nil {
		return idpsession.PendingAuthorize{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[id]
	if !ok || time.Now().After(p.ExpiresAt) {
		delete(s.pending, id)
		return idpsession.PendingAuthorize{}, fmt.Errorf("memory idp pending: %w", idpsession.ErrNotFound)
	}
	delete(s.pending, id)
	return p, nil
}

func randomID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("memory idp session: generate id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
