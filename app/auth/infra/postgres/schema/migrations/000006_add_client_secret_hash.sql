-- +goose Up
-- ISSUE-035: add client_secret_hash to clients table to support confidential
-- clients (RFC 6749 2.1). The column is nullable: NULL represents a public
-- client (no secret, token_endpoint_auth_method=none); a non-NULL value is
-- a bcrypt hash of the client_secret (stored like user passwords per
-- ISSUE-005 pattern, never the plaintext). No data loss: this is an additive
-- ALTER TABLE. Existing rows receive NULL (= public client), preserving
-- existing behavior.
ALTER TABLE clients ADD COLUMN client_secret_hash text;

-- +goose Down
ALTER TABLE clients DROP COLUMN client_secret_hash;
