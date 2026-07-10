-- +goose Up
-- SPEC-006 R4/R5/R8: refresh_tokens backs domain/refreshtoken.Repository
-- (docs/plans/SPEC-006-plan.md 付録 A). Like authorization_codes (see
-- 000001_create_auth.sql), this table is deliberately unqualified (no
-- schema/database prefix): auth connects to its own dedicated Postgres
-- database, so this file is byte-identical regardless of which
-- database it ends up applied to.
--
-- token_hash is the PRIMARY KEY: per R8, only the SHA-256 hex hash of
-- a refresh token's plaintext is ever persisted -- the plaintext
-- itself is returned to the caller exactly once (at Issue/Rotate) and
-- is never stored, logged, or used as a lookup key here.
--
-- family_id identifies the rotation chain a token belongs to (SPEC-006
-- 付録 A's Repository.Rotate/RevokeFamily doc comments): every token
-- produced by successive rotations from one initial Issue shares the
-- same family_id, and RevokeFamily (RFC 9700 4.14 reuse-detection
-- response) deletes every row for a family_id in one statement. The
-- companion index below is what makes that DELETE an index scan
-- instead of a sequential one.
--
-- consumed defaults to false and is flipped to true (never deleted) by
-- a successful Repository.Rotate (db/queries/refresh_tokens.sql's
-- ConsumeRefreshToken): unlike authorization_codes, refresh tokens
-- are NOT delete-based on redemption, because a consumed-but-
-- unexpired row must remain readable (see FindByTokenHash's contract)
-- so a replay of an already-rotated token can be detected as reuse
-- (RFC 9700 4.14) rather than merely reported as "not found".
--
-- expires_at is timestamptz (not timestamp), matching
-- authorization_codes.expires_at, so TTL comparisons against now() are
-- unambiguous with respect to UTC offset.
CREATE TABLE refresh_tokens (
    token_hash text        PRIMARY KEY,
    family_id  text        NOT NULL,
    client_id  text        NOT NULL,
    user_id    text        NOT NULL,
    scope      text        NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed   boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Supports RevokeFamily's `DELETE FROM refresh_tokens WHERE family_id
-- = $1` (db/queries/refresh_tokens.sql) as an index scan rather than a
-- sequential scan over the whole table.
CREATE INDEX idx_refresh_tokens_family_id ON refresh_tokens (family_id);

-- +goose Down
DROP TABLE refresh_tokens;
