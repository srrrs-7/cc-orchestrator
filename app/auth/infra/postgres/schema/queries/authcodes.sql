-- SPEC-005 R2/R4: sqlc input for the authorization_codes table
-- (schema/migrations/000001_create_auth.sql). `make sqlc` regenerates
-- infra/postgres/sqlcgen from this file; keep both in the same commit
-- (no drift).

-- name: InsertAuthCode :exec
-- Backs authcode.Repository.Save: authorization codes are
-- issue-once/consume-once, so this is a plain INSERT (not an upsert --
-- a code colliding with an existing primary key would indicate a
-- broken random generator, not a legitimate re-save).
-- auth_time ($10) is the OIDC IdP session login timestamp; NULL means
-- not available (maps to time.Time{} in Go -- see ISSUE-038).
INSERT INTO authorization_codes (
    code, client_id, user_id, redirect_uri, scope, nonce,
    challenge, challenge_method, expires_at, consumed, auth_time
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, false, $10
);

-- name: GetActiveAuthCode :one
-- Backs authcode.Repository.FindByCode. Only a row that is both
-- unconsumed and not yet past its TTL is "active"; an expired row is
-- invisible here even though it still physically exists until
-- DeleteExpiredAuthCode lazily removes it (matching
-- infra/memory.AuthCodeRepository.FindByCode's "expired looks like
-- not-found" contract). Returns sql.ErrNoRows when no active row
-- matches; infra/postgres/authcode_repository.go maps that to
-- authcode.ErrNotFound.
SELECT code, client_id, user_id, redirect_uri, scope, nonce,
       challenge, challenge_method, expires_at, consumed, auth_time
FROM authorization_codes
WHERE code = $1 AND consumed = false AND expires_at > now();

-- name: DeleteExpiredAuthCode :exec
-- Lazy eviction companion to GetActiveAuthCode: called by
-- infra/postgres/authcode_repository.go after GetActiveAuthCode finds
-- no active row, to opportunistically remove a code that exists but
-- has expired, so expired codes do not accumulate. The expires_at
-- <= now() guard makes this a no-op (not an error) if code does not
-- exist or has not actually expired.
DELETE FROM authorization_codes
WHERE code = $1 AND expires_at <= now();

-- name: PurgeExpiredAuthCodes :execrows
-- Bulk eviction companion to DeleteExpiredAuthCode (ISSUE-015): deletes ALL
-- rows whose TTL has elapsed in a single statement. Called by the background
-- purge ticker in cmd/authz/main.go (every 15 min) so expired authorization
-- codes do not accumulate between lazy-eviction opportunities.
-- Returns the number of rows deleted (sqlc :execrows).
-- Safe to call at any time: the WHERE guard makes it a no-op when no expired
-- rows exist, and it never touches unconsumed/unexpired codes.
DELETE FROM authorization_codes WHERE expires_at <= now();

-- name: ConsumeAuthCode :one
-- Backs authcode.Repository.Consume: the sole, atomic single-use
-- mechanism (plan §0 "authcode 単回使用/TTL"). DELETE ... RETURNING is
-- a single statement, so Postgres's own row-level locking guarantees
-- that when two callers race to consume the same code, exactly one
-- DELETE removes (and thus "wins") the row; the loser's statement
-- affects zero rows. Returns sql.ErrNoRows (0 rows deleted) when no
-- row with this code exists at all -- this covers both a genuinely
-- unknown code and a repeat consume of an already-consumed
-- (already-deleted) one, which infra/postgres/authcode_repository.go
-- maps to authcode.ErrNotFound. When a row IS deleted, the caller
-- compares the returned expires_at against time.Now() to distinguish
-- a valid consume (nil) from a consume of an expired-but-not-yet-
-- lazily-evicted row (authcode.ErrExpired), matching
-- infra/memory.AuthCodeRepository.Consume's expiry check.
DELETE FROM authorization_codes
WHERE code = $1
RETURNING expires_at;

-- name: DeleteAuthCodesByUserID :exec
DELETE FROM authorization_codes WHERE user_id = $1;

-- name: DeleteAuthCodesByClientID :exec
DELETE FROM authorization_codes WHERE client_id = $1;
