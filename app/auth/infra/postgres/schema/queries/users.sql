-- SPEC-005 R2/R4: sqlc input for the users table
-- (schema/migrations/000001_create_auth.sql). `make sqlc` regenerates
-- infra/postgres/sqlcgen from this file; keep both in the same commit
-- (no drift).

-- name: GetUserByID :one
-- Backs user.Repository.FindByID. Returns sql.ErrNoRows when absent;
-- infra/postgres/user_repository.go maps that to user.ErrNotFound.
SELECT id, username, password_hash, profile_name, profile_email
FROM users
WHERE id = $1;

-- name: GetUserByUsername :one
-- Backs user.Repository.FindByUsername. username is UNIQUE (see the
-- migration), so at most one row ever matches. Returns sql.ErrNoRows
-- when absent; infra/postgres/user_repository.go maps that to
-- user.ErrNotFound.
SELECT id, username, password_hash, profile_name, profile_email
FROM users
WHERE username = $1;

-- name: ListUsers :many
-- Backs user.Repository.ListAll for the admin management API.
SELECT id, username, password_hash, profile_name, profile_email
FROM users
ORDER BY id;

-- name: DeleteUser :execrows
-- Removes a user row. Returns 0 rows when id is absent.
DELETE FROM users WHERE id = $1;

-- name: UpsertUser :exec
-- Backs the startup idempotent seed (infra/postgres/seed.go's
-- SeedUser), not user.Repository itself (which is read-only). Inserts
-- a new row, or overwrites every column in place when id already
-- exists, so repeated process starts converge on the same seed data
-- rather than erroring on the second run.
INSERT INTO users (id, username, password_hash, profile_name, profile_email)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
    username       = EXCLUDED.username,
    password_hash  = EXCLUDED.password_hash,
    profile_name   = EXCLUDED.profile_name,
    profile_email  = EXCLUDED.profile_email;
