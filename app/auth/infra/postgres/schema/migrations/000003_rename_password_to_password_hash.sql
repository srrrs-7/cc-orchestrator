-- +goose Up
-- ISSUE-005: store bcrypt hashes instead of plaintext passwords.
-- SeedUser upserts overwrite demo rows on every startup, so no data
-- backfill is required for local/dev environments.
ALTER TABLE users RENAME COLUMN password TO password_hash;

-- +goose Down
ALTER TABLE users RENAME COLUMN password_hash TO password;
