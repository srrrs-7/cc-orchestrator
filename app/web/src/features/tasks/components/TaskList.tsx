import { useSearch } from "@tanstack/react-router";
import { useMemo } from "react";
import { filterByStatus, sortByPriority } from "../domain/task";
import { useTasksQuery } from "../hooks/useTasks";
import { TaskItem } from "./TaskItem";
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
    return <p className="text-sm text-gray-500">Loading tasks...</p>;
  }

  if (isError) {
    return (
      <p role="alert" className="text-sm text-red-600">
        Failed to load tasks: {error.message}
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {tasks.length === 0 ? (
        <p className="text-sm text-gray-500">No tasks found.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {tasks.map((task) => (
            <li key={task.id}>
              <TaskItem task={task} />
            </li>
          ))}
        </ul>
      )}
      {data !== undefined && <TaskPager page={data} />}
    </div>
  );
}
