package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/user"
)

// UserInfoService implements the OIDC UserInfo endpoint use case
// (OIDC Core 5.3): given a bearer access token, verify it and return
// the claims about its subject that the token's granted scope
// permits.
type UserInfoService struct {
	users    user.Repository
	verifier token.Verifier
	issuer   string
}

// NewUserInfoService builds a UserInfoService. issuer is used both to
// validate the access token's "iss" claim and, per this
// authorization server's audience design, its "aud" claim (see
// AuthorizationService.Token).
func NewUserInfoService(users user.Repository, verifier token.Verifier, issuer string) *UserInfoService {
	return &UserInfoService{users: users, verifier: verifier, issuer: issuer}
}

// UserInfo verifies bearerToken as an access token JWT and returns
// the UserInfo response for its subject.
//
// token.Verifier only checks the signature and "exp"; UserInfo
// additionally checks "iss" (must be this issuer) and "aud" (must
// also equal this issuer, since access tokens are minted with
// aud=issuer to scope them to this UserInfo endpoint -- see
// AuthorizationService.Token), and requires a non-empty "sub" (OIDC
// Core 5.3.2: sub is REQUIRED in the UserInfo response).
func (s *UserInfoService) UserInfo(ctx context.Context, bearerToken string) (UserInfoDTO, error) {
	claims, err := s.verifier.Verify(bearerToken)
	if err != nil {
		return UserInfoDTO{}, fmt.Errorf("service: userinfo: %w", err)
	}
	if claims.Issuer != s.issuer || claims.Audience != s.issuer || claims.Subject == "" {
		return UserInfoDTO{}, fmt.Errorf("service: userinfo: %w", token.ErrInvalidToken)
	}

	uid, err := user.ParseUserID(claims.Subject)
	if err != nil {
		return UserInfoDTO{}, fmt.Errorf("service: userinfo: %w", err)
	}
	u, err := s.users.FindByID(ctx, uid)
	if err != nil {
		return UserInfoDTO{}, fmt.Errorf("service: userinfo: %w", err)
	}

	dto := UserInfoDTO{Subject: u.ID().String()}
	for _, sc := range strings.Fields(claims.Scope) {
		switch sc {
		case "profile":
			dto.Name = u.Profile().Name()
		case "email":
			dto.Email = u.Profile().Email()
		}
	}
	return dto, nil
}
