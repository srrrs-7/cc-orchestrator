import { useSearch } from "@tanstack/react-router";
import { useMemo } from "react";
import { filterByStatus, sortByPriority } from "../domain/task";
import { useTasksQuery } from "../hooks/useTasks";
import { TaskItem } from "./TaskItem";

/**
 * Renders the task list for the currently selected status filter (the
 * status filter lives in the URL search params, read here via the
 * router). All filtering/sorting is delegated to the domain layer;
 * this component only subscribes to data and renders it.
 */
export function TaskList() {
  const { status } = useSearch({ from: "/" });
  const { data, isLoading, isError, error } = useTasksQuery();
  const tasks = useMemo(() => sortByPriority(filterByStatus(data ?? [], status)), [data, status]);

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

  if (tasks.length === 0) {
    return <p className="text-sm text-gray-500">No tasks found.</p>;
  }

  return (
    <ul className="flex flex-col gap-2">
      {tasks.map((task) => (
        <li key={task.id}>
          <TaskItem task={task} />
        </li>
      ))}
    </ul>
  );
}
