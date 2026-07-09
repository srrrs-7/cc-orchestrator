import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { completeTask, createTask, fetchTaskById, fetchTasks, startTask } from "../api/client";
import type { CreateTaskInput } from "../api/schema";
import type { Task } from "../domain/task";

const tasksQueryKey = ["tasks"] as const;

/** Server state: the full task list. Never duplicated into local state. */
export function useTasksQuery() {
  return useQuery<Task[], ApiError>({
    queryKey: tasksQueryKey,
    queryFn: fetchTasks,
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
