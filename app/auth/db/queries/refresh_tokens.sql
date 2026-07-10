-- SPEC-006 R4/R5/R8: sqlc input for the refresh_tokens table
-- (db/migrations/000002_create_refresh_tokens.sql). `make sqlc`
-- regenerates infra/postgres/sqlcgen from this file; keep both in the
-- same commit (no drift).

-- name: InsertRefreshToken :exec
-- Backs refreshtoken.Repository.Save (the initial refresh token minted
-- at authorization_code exchange) and the "insert newRT" half of
-- Rotate's atomic consume-old + insert-new transaction. It is a plain
-- INSERT, not an upsert: a token_hash collision would indicate a
-- broken random generator, not a legitimate re-save (mirrors
-- InsertAuthCode's doc comment in authcodes.sql).
INSERT INTO refresh_tokens (
    token_hash, family_id, client_id, user_id, scope, expires_at, consumed
) VALUES (
    $1, $2, $3, $4, $5, $6, false
);

-- name: GetRefreshToken :one
-- Backs refreshtoken.Repository.FindByTokenHash. Unlike
-- GetActiveAuthCode, this query is deliberately NOT filtered by
-- `consumed = false`: a consumed-but-unexpired row must still be
-- returned so the caller can detect a replay of an already-rotated
-- token (SPEC-006 付録 A's FindByTokenHash doc comment -- reuse
-- detection needs to see it before it even reaches Rotate). Only
-- expiry hides a row here; expired rows are lazily evicted by
-- DeleteExpiredRefreshToken below. Returns sql.ErrNoRows when no
-- non-expired row matches; infra/postgres/refreshtoken_repository.go
-- maps that to refreshtoken.ErrNotFound (after opportunistically
-- evicting an expired row, if any).
SELECT token_hash, family_id, client_id, user_id, scope, expires_at, consumed
FROM refresh_tokens
WHERE token_hash = $1 AND expires_at > now();

-- name: DeleteExpiredRefreshToken :exec
-- Lazy eviction companion to GetRefreshToken, mirroring
-- DeleteExpiredAuthCode: called after GetRefreshToken finds no
-- non-expired row, to opportunistically remove a row that exists but
-- has expired, so expired refresh tokens do not accumulate. The
-- expires_at <= now() guard makes this a no-op (not an error) if
-- token_hash does not exist or has not actually expired.
DELETE FROM refresh_tokens
WHERE token_hash = $1 AND expires_at <= now();

-- name: ConsumeRefreshToken :one
-- Backs the "consume old" half of refreshtoken.Repository.Rotate
-- (SPEC-006 付録 A): the sole, atomic single-use mechanism for a
-- refresh token, the RefreshToken analogue of ConsumeAuthCode.
-- UPDATE ... RETURNING is a single statement, so Postgres's own
-- row-level locking guarantees that when two callers race to rotate
-- the same token, exactly one UPDATE flips consumed (and thus "wins")
-- and every other caller's statement affects zero rows. Returns
-- sql.ErrNoRows (0 rows updated) both when token_hash does not exist
-- at all, has already expired, or -- critically -- already has
-- consumed = true (the WHERE clause's `consumed = false` guard is
-- what makes an already-consumed row invisible here, rather than
-- being re-flipped). infra/postgres/refreshtoken_repository.go
-- distinguishes these zero-rows cases (ErrReused vs ErrNotFound) with
-- a follow-up GetRefreshToken call in the same transaction, per 付録
-- A's precedence rule.
UPDATE refresh_tokens
SET consumed = true
WHERE token_hash = $1 AND consumed = false AND expires_at > now()
RETURNING token_hash;

-- name: RevokeFamilyRefreshTokens :exec
-- Backs refreshtoken.Repository.RevokeFamily: the reuse-detection
-- response (RFC 9700 4.14) that invalidates every token in a rotation
-- chain (both active and already-consumed rows) in one statement, so
-- a stolen token cannot yield further tokens. Deleting zero rows (an
-- unknown or already-empty family_id) is not an error -- idempotent,
-- matching Repository.RevokeFamily's doc comment.
DELETE FROM refresh_tokens
WHERE family_id = $1;
