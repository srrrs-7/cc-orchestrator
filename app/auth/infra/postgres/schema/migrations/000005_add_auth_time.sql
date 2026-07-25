-- +goose Up
-- ISSUE-038: add auth_time to authorization_codes and refresh_tokens so the
-- OIDC auth_time claim (IdP session login timestamp) survives a process restart
-- and is available to the token endpoint when loading codes/tokens from the DB.
--
-- The column is nullable: a SQL NULL represents "auth_time not available"
-- (e.g. rows written before this migration, or codes/tokens issued without a
-- valid IdP session timestamp). The corresponding Go value is time.Time{} (zero).
-- Nullable is chosen over NOT NULL DEFAULT '1970-01-01' to give a clean,
-- unambiguous round-trip: NULL ↔ time.Time{} via sql.NullTime, without any
-- sentinel-value detection in application code.
--
-- No data loss: this is an additive ALTER TABLE (no column removal, no type
-- change, no NOT NULL constraint tightening). Existing rows receive NULL, which
-- the application layer correctly reads back as time.Time{} (zero).
ALTER TABLE authorization_codes ADD COLUMN auth_time timestamptz;
ALTER TABLE refresh_tokens      ADD COLUMN auth_time timestamptz;

-- +goose Down
ALTER TABLE authorization_codes DROP COLUMN auth_time;
ALTER TABLE refresh_tokens      DROP COLUMN auth_time;
