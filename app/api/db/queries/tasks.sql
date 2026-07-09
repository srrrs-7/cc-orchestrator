-- SPEC-005 R1/R4: sqlc input for the tasks table (db/migrations/000001_create_tasks.sql).
-- `make sqlc` regenerates infra/postgres/sqlcgen from this file; keep
-- both in the same commit (no drift).

-- name: UpsertTask :exec
-- Backs task.Repository.Save: insert a new row, or update every
-- mutable column in place when id already exists (matching
-- infra/memory.TaskRepository.Save's upsert-by-id semantics -- see
-- infra/repotest.RunTaskRepositoryContract's "Save called again with
-- the same id upserts" subtest). created_at is intentionally excluded
-- from the DO UPDATE SET list: it must not change on update.
INSERT INTO tasks (id, title, status, priority, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    title      = EXCLUDED.title,
    status     = EXCLUDED.status,
    priority   = EXCLUDED.priority,
    updated_at = EXCLUDED.updated_at;

-- name: GetTaskByID :one
-- Backs task.Repository.FindByID. Returns sql.ErrNoRows when absent;
-- infra/postgres/task_repository.go maps that to task.ErrNotFound.
SELECT id, title, status, priority, created_at, updated_at
FROM tasks
WHERE id = $1;

-- name: GetTaskByTitle :one
-- Backs task.Repository.FindByTitle. Returns sql.ErrNoRows when
-- absent; infra/postgres/task_repository.go maps that to
-- task.ErrNotFound.
SELECT id, title, status, priority, created_at, updated_at
FROM tasks
WHERE title = $1;

-- name: ListTasks :many
-- Backs task.Repository.FindAll. Ordered by created_at for stable,
-- deterministic output (infra/memory has no inherent order since it
-- ranges over a map).
SELECT id, title, status, priority, created_at, updated_at
FROM tasks
ORDER BY created_at, id;
