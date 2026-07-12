package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
)

// RevokeRequest is the application-layer input for token revocation
// (RFC 7009). ClientSecret is the plaintext secret for confidential
// clients (ISSUE-035); empty for public clients.
type RevokeRequest struct {
	Token         string
	TokenTypeHint string
	ClientID      string
	ClientSecret  string
}

// Revoke revokes the presented token when it is a refresh token. Unknown
// or already-revoked tokens are treated as success per RFC 7009 §2.2.
// Access tokens (JWT) are not blocklisted in this sample.
func (s *AuthorizationService) Revoke(ctx context.Context, req RevokeRequest) error {
	if req.Token == "" {
		return nil
	}

	hash := refreshtoken.HashToken(req.Token)
	rt, err := s.refreshTokens.FindByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, refreshtoken.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("service: revoke: %w", err)
	}

	tokenClientID, err := client.ParseClientID(rt.ClientID().String())
	if err != nil {
		return nil
	}
	c, err := s.clients.FindByID(ctx, tokenClientID)
	if err != nil {
		return nil
	}

	if c.IsConfidential() {
		// RFC 7009 §2.1: confidential clients MUST authenticate at revoke.
		presentedCID, parseErr := client.ParseClientID(req.ClientID)
		if req.ClientID == "" || parseErr != nil ||
			presentedCID.String() != tokenClientID.String() ||
			!c.VerifySecret(req.ClientSecret) {
			return nil
		}
	} else if req.ClientID != "" {
		presentedCID, parseErr := client.ParseClientID(req.ClientID)
		if parseErr != nil {
			return nil
		}
		if err := rt.MatchesClient(refreshtoken.NewClientID(presentedCID.String())); err != nil {
			return nil
		}
	}

	if err := s.refreshTokens.RevokeFamily(ctx, rt.FamilyID()); err != nil {
		return fmt.Errorf("service: revoke: %w", err)
	}
	return nil
}
