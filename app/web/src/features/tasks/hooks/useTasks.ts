import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { createTask, fetchTasks, updateTaskStatus } from "../api/client";
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
