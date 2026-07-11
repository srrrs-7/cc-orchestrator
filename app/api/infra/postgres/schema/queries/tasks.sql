-- SPEC-005 R1/R4: sqlc input for the tasks table (schema/migrations/000001_create_tasks.sql).
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

-- name: ListTasksPage :many
-- Backs task.Repository.ListPage (SPEC-008 R1/R2/R5): a single page of
-- tasks ordered by created_at, id for stable, deterministic output
-- (ties on created_at are broken by id so page boundaries never
-- duplicate or skip a row; infra/memory.TaskRepository.ListPage sorts
-- the same way). $1 = limit (task.Page.Limit(), already clamped to
-- task.MaxLimit by domain/task.NewPage), $2 = offset
-- (task.Page.Offset()). An offset at or beyond the table's row count
-- simply yields zero rows -- not an error. Both are cast to ::bigint
-- so sqlc generates int64 params (ListTasksPageParams.Limit/Offset):
-- Go's int -> int64 is a widening conversion on every platform this
-- runs on, so infra/postgres/task_repository.go never narrows
-- task.Page.Limit()/Offset() into a smaller type (gosec G115).
SELECT id, title, status, priority, created_at, updated_at
FROM tasks
ORDER BY created_at, id
LIMIT sqlc.arg('limit')::bigint OFFSET sqlc.arg('offset')::bigint;

-- name: CountTasks :one
-- Backs task.Repository.ListPage's total (SPEC-008 R2): the total
-- number of tasks regardless of limit/offset, so callers can compute
-- page count / whether further pages exist.
SELECT COUNT(*) FROM tasks;
