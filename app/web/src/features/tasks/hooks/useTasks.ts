import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { createTask, fetchTaskById, fetchTasks, updateTaskStatus } from "../api/client";
import type { CreateTaskInput } from "../api/schema";
import type { Task, TaskStatus } from "../domain/task";

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

type UpdateTaskStatusInput = {
  readonly id: string;
  readonly status: TaskStatus;
};

export function useUpdateTaskStatus() {
  const queryClient = useQueryClient();
  return useMutation<Task, ApiError, UpdateTaskStatusInput>({
    mutationFn: ({ id, status }) => updateTaskStatus(id, status),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tasksQueryKey });
    },
  });
}
