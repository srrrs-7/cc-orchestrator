package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/sqlcgen"
)

// RefreshTokenRepository is a Postgres-backed implementation of
// refreshtoken.Repository (SPEC-006 R4/R5/R8): single-use rotation and
// reuse detection are enforced by a single transaction per Rotate
// call (see Rotate's doc comment), so the atomicity guarantee holds
// across every process/instance sharing the same database -- the same
// gap SPEC-005's AuthCodeRepository closes for authorization codes.
//
// db (in addition to q, the non-transactional *sqlcgen.Queries handle
// used by Save/FindByTokenHash/RevokeFamily) is kept so Rotate can
// open its own *sql.Tx: none of this repository's other methods need
// a transaction, but Rotate's "consume old, insert new" pair must
// commit or roll back together (see its doc comment).
type RefreshTokenRepository struct {
	q  *sqlcgen.Queries
	db *sql.DB
}

// var _ refreshtoken.Repository = (*RefreshTokenRepository)(nil)
// verifies at compile time that RefreshTokenRepository satisfies the
// domain's Repository interface.
var _ refreshtoken.Repository = (*RefreshTokenRepository)(nil)

// NewRefreshTokenRepository builds a RefreshTokenRepository backed by
// db.
func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{q: sqlcgen.New(db), db: db}
}

// Save inserts rt as a new row (the initial refresh token minted at
// authorization_code exchange, refreshtoken.Issue). Like
// AuthCodeRepository.Save, this is a plain INSERT, not an upsert: a
// token_hash collision would indicate a broken random generator, not
// a legitimate re-save.
func (r *RefreshTokenRepository) Save(ctx context.Context, rt *refreshtoken.RefreshToken) error {
	if err := r.q.InsertRefreshToken(ctx, insertRefreshTokenParams(rt)); err != nil {
		return fmt.Errorf("postgres: save refresh token: %w", err)
	}
	return nil
}

// FindByTokenHash returns the RefreshToken stored under hash. A
// consumed-but-unexpired row is returned on purpose (db/queries's
// GetRefreshToken query is not filtered by consumed), so callers can
// detect a reuse of an already-rotated token before it even reaches
// Rotate. It returns a wrapped refreshtoken.ErrNotFound when no row
// exists, or the row has expired; an expired row is opportunistically
// evicted (lazy deletion) as a side effect, matching
// infra/memory.RefreshTokenRepository.FindByTokenHash.
func (r *RefreshTokenRepository) FindByTokenHash(ctx context.Context, hash refreshtoken.TokenHash) (*refreshtoken.RefreshToken, error) {
	row, err := r.q.GetRefreshToken(ctx, hash.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The row might still physically exist but be expired
			// (GetRefreshToken's WHERE clause hides it either way);
			// opportunistically delete it so expired tokens do not
			// accumulate. No-op if the hash genuinely does not exist or
			// has not actually expired (see DeleteExpiredRefreshToken's
			// WHERE guard).
			if delErr := r.q.DeleteExpiredRefreshToken(ctx, hash.String()); delErr != nil {
				return nil, fmt.Errorf("postgres: find refresh token: evict expired: %w", delErr)
			}
			return nil, fmt.Errorf("postgres: find refresh token: %w", refreshtoken.ErrNotFound)
		}
		return nil, fmt.Errorf("postgres: find refresh token: %w", err)
	}

	rt, err := reconstructRefreshToken(
		row.TokenHash, row.FamilyID, row.ClientID, row.UserID, row.Scope,
		row.ExpiresAt, row.Consumed,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: find refresh token: %w", err)
	}
	return rt, nil
}

// Rotate atomically consumes the token identified by oldHash and
// inserts newRT within a single *sql.Tx (SPEC-006 付録 A, the
// RefreshToken analogue of AuthCodeRepository.Consume's single
// DELETE ... RETURNING statement): ConsumeRefreshToken's
// UPDATE ... WHERE consumed = false AND expires_at > now() ...
// RETURNING is what makes exactly one concurrent caller "win" a race
// to rotate the same token (Postgres's own row-level locking
// serializes the two UPDATEs against the same row), while everything
// else in this method (the follow-up GetRefreshToken precedence check
// and the INSERT of newRT) runs in the same transaction so a losing
// or erroring path leaves the store completely unchanged (no partial
// write) and the winning path's consume+insert commit together.
//
//   - ConsumeRefreshToken affects exactly one row -> INSERT newRT ->
//     commit -> nil;
//   - ConsumeRefreshToken affects zero rows (sql.ErrNoRows) -> a
//     follow-up GetRefreshToken (same tx, same WHERE token_hash = $1
//     AND expires_at > now() as FindByTokenHash) distinguishes why: a
//     row is still found -> it must be already consumed (that is the
//     only way UPDATE could have matched zero rows for a non-expired,
//     existing token_hash) -> ErrReused; no row found -> absent or
//     expired -> ErrNotFound. Either way the transaction is rolled
//     back (via the deferred tx.Rollback(), a no-op once Commit has
//     already succeeded on the nil path) rather than committed, so no
//     row is ever left half-updated.
func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldHash refreshtoken.TokenHash, newRT *refreshtoken.RefreshToken) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: rotate refresh token: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit has succeeded

	qtx := r.q.WithTx(tx)

	if _, err := qtx.ConsumeRefreshToken(ctx, oldHash.String()); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("postgres: rotate refresh token: consume: %w", err)
		}

		if _, getErr := qtx.GetRefreshToken(ctx, oldHash.String()); getErr != nil {
			if errors.Is(getErr, sql.ErrNoRows) {
				return fmt.Errorf("postgres: rotate refresh token: %w", refreshtoken.ErrNotFound)
			}
			return fmt.Errorf("postgres: rotate refresh token: precedence check: %w", getErr)
		}
		return fmt.Errorf("postgres: rotate refresh token: %w", refreshtoken.ErrReused)
	}

	if err := qtx.InsertRefreshToken(ctx, insertRefreshTokenParams(newRT)); err != nil {
		return fmt.Errorf("postgres: rotate refresh token: insert new: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgres: rotate refresh token: commit: %w", err)
	}
	return nil
}

// RevokeFamily deletes every refresh token whose family_id matches
// familyID in a single statement (RevokeFamilyRefreshTokens). Deleting
// zero rows is not an error -- idempotent, matching
// refreshtoken.Repository.RevokeFamily's doc comment.
func (r *RefreshTokenRepository) RevokeFamily(ctx context.Context, familyID refreshtoken.FamilyID) error {
	if err := r.q.RevokeFamilyRefreshTokens(ctx, familyID.String()); err != nil {
		return fmt.Errorf("postgres: revoke refresh token family: %w", err)
	}
	return nil
}

// insertRefreshTokenParams projects a *refreshtoken.RefreshToken onto
// InsertRefreshToken's parameter struct, shared by Save (a
// non-transactional insert) and Rotate (a transactional one).
func insertRefreshTokenParams(rt *refreshtoken.RefreshToken) sqlcgen.InsertRefreshTokenParams {
	return sqlcgen.InsertRefreshTokenParams{
		TokenHash: rt.TokenHash().String(),
		FamilyID:  rt.FamilyID().String(),
		ClientID:  rt.ClientID().String(),
		UserID:    rt.UserID().String(),
		Scope:     rt.Scope().String(),
		ExpiresAt: rt.ExpiresAt(),
	}
}

// reconstructRefreshToken rebuilds a *refreshtoken.RefreshToken from
// persisted column values, re-validating each through the domain's
// own constructors (refreshtoken.ParseTokenHash / ParseFamilyID /
// ParseScope) so a row that could not have been produced by this
// repository's own Save/Rotate is surfaced as an error rather than
// silently accepted (mirrors reconstructAuthCode in
// authcode_repository.go).
func reconstructRefreshToken(
	hashStr, familyIDStr, clientIDStr, userIDStr, scopeStr string,
	expiresAt time.Time,
	consumed bool,
) (*refreshtoken.RefreshToken, error) {
	hash, err := refreshtoken.ParseTokenHash(hashStr)
	if err != nil {
		return nil, fmt.Errorf("parse token hash: %w", err)
	}
	familyID, err := refreshtoken.ParseFamilyID(familyIDStr)
	if err != nil {
		return nil, fmt.Errorf("parse family id: %w", err)
	}
	scope, err := refreshtoken.ParseScope(scopeStr)
	if err != nil {
		return nil, fmt.Errorf("parse scope: %w", err)
	}

	return refreshtoken.Reconstruct(
		hash,
		familyID,
		refreshtoken.NewClientID(clientIDStr),
		refreshtoken.NewUserID(userIDStr),
		scope,
		expiresAt,
		consumed,
	), nil
}
