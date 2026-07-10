import { z } from "zod";
import type { PageInfo } from "../domain/pagination";
import type { Task, TaskPriority } from "../domain/task";
import { TASK_PRIORITIES } from "../domain/task";
import type {
  RouteChangePriorityRequest,
  RouteCreateTaskRequest,
  RouteErrorResponse,
  RouteTaskListResponse,
  RouteTaskResponse,
} from "./generated";
import {
  zRouteChangePriorityRequest,
  zRouteCreateTaskRequest,
  zRouteErrorResponse,
  zRouteTaskListResponse,
  zRouteTaskResponse,
} from "./generated/zod.gen";

// Re-export the generated wire types/schemas so the rest of the feature
// imports the contract from a single place (`./schema`) instead of
// reaching into `./generated` directly.
export type {
  RouteChangePriorityRequest,
  RouteCreateTaskRequest,
  RouteErrorResponse,
  RouteTaskListResponse,
  RouteTaskResponse,
};
export {
  zRouteChangePriorityRequest,
  zRouteCreateTaskRequest,
  zRouteErrorResponse,
  zRouteTaskListResponse,
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

/**
 * Wire format for `GET /tasks` (SPEC-008): a paging envelope, not a
 * bare array. `route.taskListResponse` (openapi.yaml) marks every
 * property `required`, so the generated schema is re-exported as-is,
 * the same way `taskSchema` re-exports `zRouteTaskResponse` above.
 */
export const taskListSchema = zRouteTaskListResponse;

export type TaskListDto = z.infer<typeof taskListSchema>;

/**
 * Domain-facing shape for a page of tasks: the wire envelope's items
 * mapped to domain `Task`s, plus the pagination metadata the server
 * actually applied (`PageInfo`, domain/pagination.ts). `limit`/`offset`
 * here are the *echoed* values (e.g. after server-side clamping), not
 * necessarily what was requested.
 */
export type TaskPage = PageInfo & {
  readonly items: readonly Task[];
};

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

/** Envelope DTO (wire format) -> domain TaskPage. */
export function toDomainPage(dto: RouteTaskListResponse): TaskPage {
  return {
    items: dto.items.map(toDomain),
    total: dto.total,
    limit: dto.limit,
    offset: dto.offset,
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
