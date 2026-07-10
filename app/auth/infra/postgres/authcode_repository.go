package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// AuthCodeRepository is a Postgres-backed implementation of
// authcode.Repository (SPEC-005 R2): single-use redemption and TTL
// expiry are enforced by the SQL itself (db/queries/authcodes.sql),
// not by application-level locking, so the guarantees hold across
// every process/instance sharing the same database -- the concrete
// gap this Spec closes relative to infra/memory's single-instance
// map.
type AuthCodeRepository struct {
	q *sqlcgen.Queries
}

// var _ authcode.Repository = (*AuthCodeRepository)(nil) verifies at
// compile time that AuthCodeRepository satisfies the domain's
// Repository interface. The narrower var _ authcode.Reader / var _
// authcode.Writer assertions below additionally pin that
// AuthCodeRepository satisfies each half of SPEC-010's Reader/Writer
// split on its own -- it stays a single struct (unlike task's
// TaskReader/TaskWriter split) because this Spec's fixed wiring
// decision constructs it with the writer pool only, for both reads
// and writes (see cmd/authz/main.go and
// docs/plans/SPEC-010-plan.md's "auth の correctness-critical read の
// 配置").
var (
	_ authcode.Repository = (*AuthCodeRepository)(nil)
	_ authcode.Reader     = (*AuthCodeRepository)(nil)
	_ authcode.Writer     = (*AuthCodeRepository)(nil)
)

// NewAuthCodeRepository builds an AuthCodeRepository backed by db.
// SPEC-010 does not change this constructor's shape: the composition
// root is responsible for always passing it the writer pool (never
// the reader pool), since authcode's reads are correctness-critical
// (see the Reader var _ assertion's doc comment above).
func NewAuthCodeRepository(db *sql.DB) *AuthCodeRepository {
	return &AuthCodeRepository{q: sqlcgen.New(db)}
}

// Save inserts ac as a new row. Authorization codes are issue-once
// (see db/queries/authcodes.sql's InsertAuthCode doc comment): a
// primary-key collision here would indicate a broken random
// generator, not a legitimate re-save, so this is a plain INSERT, not
// an upsert.
func (r *AuthCodeRepository) Save(ctx context.Context, ac *authcode.AuthorizationCode) error {
	if err := r.q.InsertAuthCode(ctx, sqlcgen.InsertAuthCodeParams{
		Code:            ac.Code().String(),
		ClientID:        ac.ClientID().String(),
		UserID:          ac.UserID().String(),
		RedirectUri:     ac.RedirectURI().String(),
		Scope:           ac.Scope().String(),
		Nonce:           encodeNonce(ac.Nonce()),
		Challenge:       ac.Challenge().Challenge(),
		ChallengeMethod: ac.Challenge().Method().String(),
		ExpiresAt:       ac.ExpiresAt(),
	}); err != nil {
		return fmt.Errorf("postgres: save authorization code: %w", err)
	}
	return nil
}

// FindByCode returns the AuthorizationCode with the given code, or
// authcode.ErrNotFound if no *active* (unconsumed, not expired) row
// exists. If a row exists but has expired, it is opportunistically
// evicted (lazy deletion) as a side effect, matching
// infra/memory.AuthCodeRepository.FindByCode.
func (r *AuthCodeRepository) FindByCode(ctx context.Context, code authcode.Code) (*authcode.AuthorizationCode, error) {
	row, err := r.q.GetActiveAuthCode(ctx, code.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The row might still physically exist but be expired
			// (GetActiveAuthCode's WHERE clause hides it either way);
			// opportunistically delete it so expired codes do not
			// accumulate. This is a no-op if the code genuinely does not
			// exist or has not actually expired (see DeleteExpiredAuthCode's
			// WHERE guard).
			if delErr := r.q.DeleteExpiredAuthCode(ctx, code.String()); delErr != nil {
				return nil, fmt.Errorf("postgres: find authorization code: evict expired: %w", delErr)
			}
			return nil, fmt.Errorf("postgres: find authorization code: %w", authcode.ErrNotFound)
		}
		return nil, fmt.Errorf("postgres: find authorization code: %w", err)
	}

	ac, err := reconstructAuthCode(
		row.Code, row.ClientID, row.UserID, row.RedirectUri, row.Scope,
		row.Nonce, row.Challenge, row.ChallengeMethod, row.ExpiresAt, row.Consumed,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: find authorization code: %w", err)
	}
	return ac, nil
}

// Consume atomically claims code for one-time use via a single
// DELETE ... RETURNING statement (db/queries/authcodes.sql's
// ConsumeAuthCode): Postgres's own row-level locking guarantees that
// when two callers race to consume the same code, exactly one DELETE
// removes the row and every other caller's statement affects zero
// rows, satisfying domain/authcode/repository.go's atomicity
// requirement without any application-level mutex.
//
//   - 0 rows deleted (sql.ErrNoRows) -> authcode.ErrNotFound (covers
//     both a genuinely unknown code and a repeat consume of an
//     already-consumed/already-deleted one -- see
//     repotest.RunAuthCodeRepositoryContract's "not ErrAlreadyConsumed"
//     subtest);
//   - 1 row deleted, but its expires_at was already in the past ->
//     authcode.ErrExpired (the row is deleted either way -- same
//     lazy-eviction behavior as FindByCode, per
//     domain/authcode/repository.go's Consume doc comment);
//   - 1 row deleted, expires_at in the future -> nil.
func (r *AuthCodeRepository) Consume(ctx context.Context, code authcode.Code) error {
	expiresAt, err := r.q.ConsumeAuthCode(ctx, code.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("postgres: consume authorization code: %w", authcode.ErrNotFound)
		}
		return fmt.Errorf("postgres: consume authorization code: %w", err)
	}
	if time.Now().After(expiresAt) {
		return fmt.Errorf("postgres: consume authorization code: %w", authcode.ErrExpired)
	}
	return nil
}

// reconstructAuthCode rebuilds a *authcode.AuthorizationCode from
// persisted column values, re-validating each through the domain's
// own constructors (authcode.ParseCode / authcode.ParseScope /
// authcode.ParseCodeChallengeMethod / authcode.NewCodeChallenge) so a
// row that could not have been produced by this repository's own
// Save is surfaced as an error rather than silently accepted.
func reconstructAuthCode(
	codeStr, clientIDStr, userIDStr, redirectURIStr, scopeStr string,
	nonce sql.NullString,
	challengeStr, methodStr string,
	expiresAt time.Time,
	consumed bool,
) (*authcode.AuthorizationCode, error) {
	code, err := authcode.ParseCode(codeStr)
	if err != nil {
		return nil, fmt.Errorf("parse code: %w", err)
	}
	scope, err := authcode.ParseScope(scopeStr)
	if err != nil {
		return nil, fmt.Errorf("parse scope: %w", err)
	}
	method, err := authcode.ParseCodeChallengeMethod(methodStr)
	if err != nil {
		return nil, fmt.Errorf("parse code challenge method: %w", err)
	}
	challenge, err := authcode.NewCodeChallenge(challengeStr, method)
	if err != nil {
		return nil, fmt.Errorf("new code challenge: %w", err)
	}

	return authcode.Reconstruct(
		code,
		authcode.NewClientID(clientIDStr),
		authcode.NewUserID(userIDStr),
		authcode.NewRedirectURI(redirectURIStr),
		scope,
		authcode.NewNonce(decodeNonce(nonce)),
		challenge,
		expiresAt,
		consumed,
	), nil
}

// encodeNonce maps authcode.Nonce's "empty string means no nonce"
// convention (see nonce.go's IsEmpty) onto the authorization_codes
// table's nullable nonce column: an empty Nonce is stored as SQL
// NULL, not an empty string, so the round trip through sqlc's
// sql.NullString is unambiguous.
func encodeNonce(n authcode.Nonce) sql.NullString {
	if n.IsEmpty() {
		return sql.NullString{}
	}
	return sql.NullString{String: n.String(), Valid: true}
}

// decodeNonce is encodeNonce's inverse: SQL NULL decodes back to "",
// authcode.NewNonce's canonical "no nonce" value.
func decodeNonce(n sql.NullString) string {
	if !n.Valid {
		return ""
	}
	return n.String
}
