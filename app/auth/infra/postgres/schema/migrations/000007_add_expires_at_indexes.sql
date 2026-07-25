-- +goose Up
-- ISSUE-015 / ISSUE-019: migration 000007.
-- Add expires_at indexes to support efficient bulk purge of expired
-- authorization_codes and refresh_tokens.
--
-- The bulk-purge queries (PurgeExpiredAuthCodes / PurgeExpiredRefreshTokens,
-- schema/queries/authcodes.sql and refresh_tokens.sql) are:
--
--   DELETE FROM authorization_codes WHERE expires_at <= now()
--   DELETE FROM refresh_tokens     WHERE expires_at <= now()
--
-- Without these indexes Postgres must perform a sequential scan of the full
-- table for each scheduled purge. With them it can locate expired rows via
-- an index range scan before locking them for deletion, which keeps the
-- purge's lock footprint proportional to the number of expired rows rather
-- than the table size -- important for high-traffic authorization servers
-- where these tables can hold many live rows at any moment.
--
-- authorization_codes already has DeleteExpiredAuthCode (lazy, single-row
-- eviction) that fires on every FindByCode miss, but in practice a busy
-- server issuing many short-lived codes faster than they are exchanged will
-- accumulate expired rows that lazy eviction never reaches.  The bulk purge
-- sweeps them up every 15 minutes (see cmd/authz/main.go's purge ticker).
--
-- refresh_tokens needs the same treatment: consumed+expired rows are
-- intentionally kept until they expire (reuse detection requires seeing them
-- during their TTL), but the bulk purge can safely delete rows whose
-- expires_at is in the past regardless of their consumed flag.

CREATE INDEX idx_authorization_codes_expires_at ON authorization_codes (expires_at);

CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_refresh_tokens_expires_at;
DROP INDEX IF EXISTS idx_authorization_codes_expires_at;
