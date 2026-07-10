import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { completeTask, createTask, fetchTaskById, fetchTasks, startTask } from "../api/client";
import type { CreateTaskInput, TaskPage } from "../api/schema";
import { DEFAULT_LIMIT } from "../domain/pagination";
import type { Task } from "../domain/task";

const tasksQueryKey = ["tasks"] as const;

export type TasksListParams = {
  readonly limit?: number;
  readonly offset?: number;
};

/**
 * Server state: a page of tasks (SPEC-008). Never duplicated into local
 * state. The query key includes `limit`/`offset` so each page is
 * cached independently; mutations below invalidate the `["tasks"]`
 * prefix, which covers every page key as well as `useTaskQuery`'s
 * `["tasks", id]` keys.
 *
 * `placeholderData: keepPreviousData` keeps the previously rendered
 * page's data on screen while the next/previous page's query key
 * fetches in the background, instead of dropping to a loading state
 * on every page change.
 */
export function useTasksQuery(params: TasksListParams = {}) {
  const limit = params.limit ?? DEFAULT_LIMIT;
  const offset = params.offset ?? 0;
  return useQuery<TaskPage, ApiError>({
    queryKey: [...tasksQueryKey, "list", { limit, offset }] as const,
    queryFn: () => fetchTasks({ limit, offset }),
    placeholderData: keepPreviousData,
  });
}

/**
 * Server state: a single task by id. Uses ["tasks", id] as its query
 * key, which is a prefix match of tasksQueryKey (["tasks"]), so the
 * list mutations below (which invalidate ["tasks"]) also invalidate
 * this query.
 */
export function useTaskQuery(id: string) {
  return useQuery<Task, ApiError>({
    queryKey: [...tasksQueryKey, id] as const,
    queryFn: () => fetchTaskById(id),
  });
}

export function useCreateTask() {
  const queryClient = useQueryClient();
  return useMutation<Task, ApiError, CreateTaskInput>({
    mutationFn: createTask,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tasksQueryKey });
    },
  });
}

/**
 * POST /tasks/:id/start (D2: replaces the old `PATCH .../status`
 * mutation for the todo -> doing transition).
 */
export function useStartTask() {
  const queryClient = useQueryClient();
  return useMutation<Task, ApiError, string>({
    mutationFn: startTask,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tasksQueryKey });
    },
  });
}

/**
 * POST /tasks/:id/complete (D2: replaces the old `PATCH .../status`
 * mutation for the doing -> done transition).
 */
export function useCompleteTask() {
  const queryClient = useQueryClient();
  return useMutation<Task, ApiError, string>({
    mutationFn: completeTask,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tasksQueryKey });
    },
  });
}
