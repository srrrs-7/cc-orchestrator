import { useSearch } from "@tanstack/react-router";
import { useMemo } from "react";
import { Alert } from "../../../shared/ui/Alert";
import { filterByStatus, sortByPriority } from "../domain/task";
import { useTasksQuery } from "../hooks/useTasks";
import { TaskEmptyState } from "./TaskEmptyState";
import { TaskItem } from "./TaskItem";
import { TaskListSkeleton } from "./TaskListSkeleton";
import { TaskPager } from "./TaskPager";

/**
 * Renders the current page of tasks for the currently selected status
 * filter (both the status filter and the `limit`/`offset` page live in
 * the URL search params, read here via the router; SPEC-008). All
 * filtering/sorting/paging math is delegated to the domain layer; this
 * component only subscribes to data and renders it.
 *
 * Note (SPEC-008 R-5, known limitation): `filterByStatus` only sees the
 * tasks on the current server page, not every task -- a status with
 * matches on another page can render as "No tasks found." here. Fixing
 * that would require server-side status filtering, which is out of
 * scope for this Spec.
 */
export function TaskList() {
  const { status, limit, offset } = useSearch({ from: "/" });
  const { data, isLoading, isError, error } = useTasksQuery({ limit, offset });
  const tasks = useMemo(
    () => sortByPriority(filterByStatus(data?.items ?? [], status)),
    [data, status],
  );

  if (isLoading) {
    return <TaskListSkeleton />;
  }

  if (isError) {
    return <Alert>Failed to load tasks: {error.message}</Alert>;
  }

  return (
    <section aria-labelledby="task-list-heading" className="flex flex-col gap-4">
      <h2 id="task-list-heading" className="text-base font-semibold text-gray-900">
        Tasks
      </h2>
      {tasks.length === 0 ? (
        <TaskEmptyState status={status} total={data?.total ?? 0} />
      ) : (
        <ul className="flex flex-col gap-3">
          {tasks.map((task) => (
            <li key={task.id}>
              <TaskItem task={task} />
            </li>
          ))}
        </ul>
      )}
      {data !== undefined && <TaskPager page={data} />}
    </section>
  );
}
