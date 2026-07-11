-- SPEC-005 R2/R4: sqlc input for the clients table
-- (schema/migrations/000001_create_auth.sql). `make sqlc` regenerates
-- infra/postgres/sqlcgen from this file; keep both in the same commit
-- (no drift).

-- name: GetClientByID :one
-- Backs client.Repository.FindByID. Returns sql.ErrNoRows when
-- absent; infra/postgres/client_repository.go maps that to
-- client.ErrNotFound.
SELECT id, redirect_uris, allowed_scopes, response_types, grant_types
FROM clients
WHERE id = $1;

-- name: UpsertClient :exec
-- Backs the startup idempotent seed (infra/postgres/seed.go's
-- SeedClient), not client.Repository itself (which is read-only).
-- Inserts a new row, or overwrites every column in place when id
-- already exists, so repeated process starts converge on the same
-- seed data rather than erroring on the second run.
INSERT INTO clients (id, redirect_uris, allowed_scopes, response_types, grant_types)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
    redirect_uris  = EXCLUDED.redirect_uris,
    allowed_scopes = EXCLUDED.allowed_scopes,
    response_types = EXCLUDED.response_types,
    grant_types    = EXCLUDED.grant_types;
