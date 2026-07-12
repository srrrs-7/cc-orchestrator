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

	if req.ClientID != "" {
		cid, err := client.ParseClientID(req.ClientID)
		if err != nil {
			return nil
		}
		c, err := s.clients.FindByID(ctx, cid)
		if err != nil {
			return nil
		}
		// Confidential client authentication (ISSUE-035): RFC 7009 §2.1
		// requires the client to authenticate if it is confidential.
		// Per RFC 7009 §2.2, the AS SHOULD treat an auth failure the same
		// as an invalid token (return success to not leak information).
		if c.IsConfidential() && !c.VerifySecret(req.ClientSecret) {
			return nil
		}
		if err := rt.MatchesClient(refreshtoken.NewClientID(cid.String())); err != nil {
			return nil
		}
	}

	if err := s.refreshTokens.RevokeFamily(ctx, rt.FamilyID()); err != nil {
		return fmt.Errorf("service: revoke: %w", err)
	}
	return nil
}
