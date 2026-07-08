import { httpGet, httpPatch, httpPost } from "../../../shared/api/http";
import type { Task, TaskStatus } from "../domain/task";
import type { CreateTaskInput } from "./schema";
import { taskListSchema, taskSchema, toDomain } from "./schema";

/** GET /tasks — list every task, validated and mapped to domain Task. */
export async function fetchTasks(): Promise<Task[]> {
  const json = await httpGet("/tasks");
  const dtos = taskListSchema.parse(json);
  return dtos.map(toDomain);
}

/** GET /tasks/:id — a single task, validated and mapped to domain Task. */
export async function fetchTaskById(id: string): Promise<Task> {
  const json = await httpGet(`/tasks/${encodeURIComponent(id)}`);
  const dto = taskSchema.parse(json);
  return toDomain(dto);
}

/** POST /tasks — create a task, validated and mapped to domain Task. */
export async function createTask(input: CreateTaskInput): Promise<Task> {
  const json = await httpPost("/tasks", input);
  const dto = taskSchema.parse(json);
  return toDomain(dto);
}

/** PATCH /tasks/:id/status — transition a task's status. */
export async function updateTaskStatus(id: string, status: TaskStatus): Promise<Task> {
  const json = await httpPatch(`/tasks/${encodeURIComponent(id)}/status`, { status });
  const dto = taskSchema.parse(json);
  return toDomain(dto);
}
