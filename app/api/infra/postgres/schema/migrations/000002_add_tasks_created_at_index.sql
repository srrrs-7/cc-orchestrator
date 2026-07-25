-- +goose Up
-- SPEC-008 R7 (ISSUE-008 P1): task.Repository.ListPage (SPEC-008 R1/R2/R5)
-- orders every page by `ORDER BY created_at, id` (schema/queries/tasks.sql:
-- ListTasksPage). Without a matching index, Postgres must sort the
-- entire table on every call, and a deep OFFSET additionally has to
-- scan and discard every preceding row -- both get worse as the table
-- grows. A composite btree index on (created_at, id), in the same
-- order as the ORDER BY clause, lets the planner satisfy the sort
-- (and the id tiebreak for rows sharing a created_at) directly from
-- the index instead of a full-table sort.
--
-- Plain (non-CONCURRENTLY) CREATE INDEX is used deliberately: it runs
-- inside goose's per-migration transaction (matching 000001's style),
-- which CREATE INDEX CONCURRENTLY cannot do. This sample's migration
-- path (app/migrator, run once at deploy time via an init container --
-- see .claude/rules/db.md) briefly locks writes to tasks while building
-- the index; that is an acceptable, reportable trade-off at this
-- table's expected size, and a production-grade rollout that must
-- avoid write locks would use CONCURRENTLY outside a transaction.
CREATE INDEX tasks_created_at_id_idx ON tasks (created_at, id);

-- +goose Down
DROP INDEX tasks_created_at_id_idx;
