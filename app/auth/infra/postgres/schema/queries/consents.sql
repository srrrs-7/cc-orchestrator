-- ISSUE-032: sqlc input for consents (schema/migrations/000004_create_consents.sql).

-- name: HasConsent :one
SELECT EXISTS(
    SELECT 1
    FROM consents
    WHERE user_id = $1 AND client_id = $2 AND scope = $3
) AS has_consent;

-- name: UpsertConsent :exec
INSERT INTO consents (user_id, client_id, scope)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, client_id, scope) DO UPDATE SET
    granted_at = EXCLUDED.granted_at;
