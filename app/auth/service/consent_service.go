package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/consent"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// ErrAccessDenied is returned when the resource owner rejects consent.
var ErrAccessDenied = errors.New("service: access denied")

// ConsentService tracks and evaluates user consent for OAuth scopes.
type ConsentService struct {
	consents consent.Repository
}

// NewConsentService builds a ConsentService.
func NewConsentService(consents consent.Repository) *ConsentService {
	return &ConsentService{consents: consents}
}

// HasGrant reports whether userID already granted scope to clientID.
func (s *ConsentService) HasGrant(ctx context.Context, userID user.UserID, clientID, scopeRaw string) (bool, error) {
	normalized, err := normalizeScope(scopeRaw)
	if err != nil {
		return false, fmt.Errorf("service: consent has grant: %w", err)
	}
	has, err := s.consents.HasGrant(ctx, userID.String(), clientID, normalized)
	if err != nil {
		return false, fmt.Errorf("service: consent has grant: %w", err)
	}
	return has, nil
}

// Grant records that userID approved scope for clientID.
func (s *ConsentService) Grant(ctx context.Context, userID user.UserID, clientID, scopeRaw string) error {
	normalized, err := normalizeScope(scopeRaw)
	if err != nil {
		return fmt.Errorf("service: consent grant: %w", err)
	}
	if err := s.consents.SaveGrant(ctx, userID.String(), clientID, normalized); err != nil {
		return fmt.Errorf("service: consent grant: %w", err)
	}
	return nil
}

func normalizeScope(scopeRaw string) (string, error) {
	scope, err := authcode.ParseScope(scopeRaw)
	if err != nil {
		return "", err
	}
	return scope.String(), nil
}
