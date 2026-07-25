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
	now := time.Now()
	sess := idpsession.Session{
		ID:              id,
		UserID:          userID,
		AuthenticatedAt: now,
		ExpiresAt:       now.Add(ttl),
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

func (s *IdPSessionStore) FindPendingAuthorize(ctx context.Context, id string) (idpsession.PendingAuthorize, error) {
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

// PurgeExpired removes every sessions/pending entry whose TTL has
// already elapsed as of time.Now(), and returns the total number of
// entries removed (ISSUE-044). Unlike FindSession/FindPendingAuthorize/
// ConsumePendingAuthorize's lazy, read-triggered eviction, this scans
// both maps unconditionally so entries that are never read again (the
// OOM scenario: an /authorize request abandoned before login completes)
// are still reclaimed.
//
// It intentionally is not part of the idpsession.Store domain
// interface: like postgres.AuthCodeRepository/RefreshTokenRepository's
// PurgeExpired (see cmd/authz/main.go's expiredPurger interface), this
// is an infra-layer garbage-collection concern, not a domain
// invariant. It shares that same (ctx context.Context) (int64, error)
// shape so it can be driven by the same background purge ticker.
func (s *IdPSessionStore) PurgeExpired(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	now := time.Now()
	var purged int64

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
			purged++
		}
	}
	for id, p := range s.pending {
		if now.After(p.ExpiresAt) {
			delete(s.pending, id)
			purged++
		}
	}

	return purged, nil
}

func randomID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("memory idp session: generate id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
