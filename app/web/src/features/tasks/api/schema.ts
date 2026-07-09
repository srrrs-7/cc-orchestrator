import { z } from "zod";
import type { Task, TaskPriority } from "../domain/task";
import { TASK_PRIORITIES } from "../domain/task";
import type {
  RouteChangePriorityRequest,
  RouteCreateTaskRequest,
  RouteErrorResponse,
  RouteTaskResponse,
} from "./generated";
import {
  zRouteChangePriorityRequest,
  zRouteCreateTaskRequest,
  zRouteErrorResponse,
  zRouteTaskResponse,
} from "./generated/zod.gen";

// Re-export the generated wire types/schemas so the rest of the feature
// imports the contract from a single place (`./schema`) instead of
// reaching into `./generated` directly.
export type {
  RouteChangePriorityRequest,
  RouteCreateTaskRequest,
  RouteErrorResponse,
  RouteTaskResponse,
};
export {
  zRouteChangePriorityRequest,
  zRouteCreateTaskRequest,
  zRouteErrorResponse,
  zRouteTaskResponse,
};

/**
 * Wire format for a Task, as returned by the Go API (snake_case).
 *
 * The generated `route.taskResponse` schema (openapi.yaml, produced by
 * swag from app/api's handler annotations) now marks every property
 * `required` and types `status`/`priority` as string-literal unions
 * (`"todo" | "doing" | "done"` / `"low" | "medium" | "high"`), so it is
 * re-exported as-is instead of layering a hand-written `.extend()` on
 * top (previously needed because swag omitted `required`/`enum`; see
 * git history for that version). If a future contract gap reopens
 * (e.g. a field swag can't annotate), reintroduce a minimal `.extend()`
 * here with a comment naming the specific gap it covers.
 */
export const taskSchema = zRouteTaskResponse;

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

/**
 * Domain-facing input validation for "create task" (stricter than the
 * generated `zRouteCreateTaskRequest`, which allows title/priority to be
 * missing entirely): the create-task form always submits both fields.
 */
export const createTaskRequestSchema = z.object({
  title: z.string().trim().min(1, "Title is required").max(200, "Title is too long"),
  priority: z.enum(TASK_PRIORITIES),
});

export type CreateTaskRequest = z.infer<typeof createTaskRequestSchema>;

export type CreateTaskInput = {
  readonly title: string;
  readonly priority: TaskPriority;
};
