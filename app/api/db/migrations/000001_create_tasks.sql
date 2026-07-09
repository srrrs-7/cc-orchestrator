-- +goose Up
-- SPEC-005 R1/R3: the tasks table backs domain/task.Repository. It is
-- deliberately unqualified (no schema prefix) -- the "api" schema is
-- selected purely via the connection's search_path (plan §0 "スキーマ
-- 分離機構"), so this file is byte-identical regardless of which
-- schema it ends up applied to. Schema creation itself happens outside
-- goose (local: docker/postgres/initdb, prod: iac bootstrap).
--
-- Column choices mirror the Task aggregate's value objects exactly, so
-- infra/postgres/task_repository.go can round-trip every field via
-- task.Reconstruct without any lossy mapping:
--   id         <- task.ID          (opaque string, e.g. a UUIDv4)
--   title      <- task.Title       (UNIQUE enforces the domain's "title
--                                    must be unique" invariant in depth;
--                                    see docs/plans/SPEC-005-plan.md §6.1
--                                    R-a for why infra/memory does not
--                                    enforce this at the Save() level)
--   status     <- task.Status      (CHECK mirrors task.ParseStatus's
--                                    closed set: todo/doing/done)
--   priority   <- task.Priority    (CHECK mirrors task.ParsePriority's
--                                    closed set: low/medium/high)
--   created_at/updated_at are timestamptz (not timestamp) so values are
--   stored and read back unambiguous with respect to UTC offset,
--   matching Go's time.Time semantics.
CREATE TABLE tasks (
    id         text        PRIMARY KEY,
    title      text        NOT NULL UNIQUE,
    status     text        NOT NULL CHECK (status IN ('todo', 'doing', 'done')),
    priority   text        NOT NULL CHECK (priority IN ('low', 'medium', 'high')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE tasks;
