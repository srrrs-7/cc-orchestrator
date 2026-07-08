import { z } from "zod";
import type { Task, TaskPriority } from "../domain/task";
import { TASK_PRIORITIES, TASK_STATUSES } from "../domain/task";

/**
 * Wire format for a Task, as returned by the Go API (snake_case).
 * z.enum infers the exact literal unions used by the domain layer, so
 * no `as` cast is needed to bridge dto.status -> TaskStatus.
 */
export const taskSchema = z.object({
  id: z.string().min(1),
  title: z.string(),
  status: z.enum(TASK_STATUSES),
  priority: z.enum(TASK_PRIORITIES),
  created_at: z.string(),
  updated_at: z.string(),
});

export type TaskDto = z.infer<typeof taskSchema>;

export const taskListSchema = z.array(taskSchema);

/** DTO (snake_case, wire format) -> domain Task. */
export function toDomain(dto: TaskDto): Task {
  return {
    id: dto.id,
    title: dto.title,
    status: dto.status,
    priority: dto.priority,
    createdAt: dto.created_at,
    updatedAt: dto.updated_at,
  };
}

/** Domain Task -> DTO (snake_case, wire format). */
export function toDto(task: Task): TaskDto {
  return {
    id: task.id,
    title: task.title,
    status: task.status,
    priority: task.priority,
    created_at: task.createdAt,
    updated_at: task.updatedAt,
  };
}

export const createTaskRequestSchema = z.object({
  title: z.string().trim().min(1, "Title is required").max(200, "Title is too long"),
  priority: z.enum(TASK_PRIORITIES),
});

export type CreateTaskRequest = z.infer<typeof createTaskRequestSchema>;

export type CreateTaskInput = {
  readonly title: string;
  readonly priority: TaskPriority;
};
