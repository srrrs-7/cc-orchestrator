import { HttpResponse, http } from "msw";
import { z } from "zod";
import type { TaskDto } from "../features/tasks/api/schema";
import { TASK_PRIORITIES } from "../features/tasks/domain/task";

// Request bodies are external data too (they arrive as JSON over the
// wire, even in this in-memory mock), so they go through zod just
// like the real API client does.
const createTaskRequestSchema = z.object({
  title: z.string().trim().min(1),
  priority: z.enum(TASK_PRIORITIES),
});

function nowIso(): string {
  return new Date().toISOString();
}

function canTransition(from: TaskDto["status"], to: TaskDto["status"]): boolean {
  if (from === "todo" && to === "doing") return true;
  if (from === "doing" && to === "done") return true;
  return false;
}

/**
 * Shared transition handler for POST /tasks/:id/start and
 * /tasks/:id/complete (D2: the Go wire contract has no
 * `PATCH /tasks/:id/status`). Errors use the `{"error": string}`
 * envelope (D3: route.errorResponse), not `{"message": string}`.
 */
function transitionTo(rawId: string | readonly string[] | undefined, to: TaskDto["status"]) {
  const id = typeof rawId === "string" ? rawId : undefined;
  const existing = id === undefined ? undefined : tasks.find((task) => task.id === id);
  if (existing === undefined) {
    return HttpResponse.json({ error: "Task not found" }, { status: 404 });
  }

  if (!canTransition(existing.status, to)) {
    return HttpResponse.json(
      { error: `Cannot transition from "${existing.status}" to "${to}"` },
      { status: 409 },
    );
  }

  const updated: TaskDto = { ...existing, status: to, updated_at: nowIso() };
  tasks = tasks.map((task) => (task.id === id ? updated : task));

  return HttpResponse.json(updated);
}

let tasks: TaskDto[] = [
  {
    id: "1",
    title: "Set up project scaffolding",
    status: "done",
    priority: "high",
    created_at: "2026-07-01T09:00:00.000Z",
    updated_at: "2026-07-01T09:30:00.000Z",
  },
  {
    id: "2",
    title: "Design the task domain model",
    status: "doing",
    priority: "high",
    created_at: "2026-07-02T09:00:00.000Z",
    updated_at: "2026-07-03T10:00:00.000Z",
  },
  {
    id: "3",
    title: "Write onboarding docs",
    status: "todo",
    priority: "low",
    created_at: "2026-07-03T09:00:00.000Z",
    updated_at: "2026-07-03T09:00:00.000Z",
  },
  {
    id: "4",
    title: "Review pull requests",
    status: "todo",
    priority: "medium",
    created_at: "2026-07-04T09:00:00.000Z",
    updated_at: "2026-07-04T09:00:00.000Z",
  },
];

let nextId = tasks.length + 1;

export const handlers = [
  http.get("/api/tasks", () => {
    return HttpResponse.json(tasks);
  }),

  http.get("/api/tasks/:id", ({ params }) => {
    const id = params.id;
    const task = tasks.find((candidate) => candidate.id === id);
    if (task === undefined) {
      return HttpResponse.json({ error: "Task not found" }, { status: 404 });
    }
    return HttpResponse.json(task);
  }),

  http.post("/api/tasks", async ({ request }) => {
    const json: unknown = await request.json();
    const body = createTaskRequestSchema.parse(json);

    const task: TaskDto = {
      id: String(nextId),
      title: body.title,
      priority: body.priority,
      status: "todo",
      created_at: nowIso(),
      updated_at: nowIso(),
    };
    nextId += 1;
    tasks = [...tasks, task];

    return HttpResponse.json(task, { status: 201 });
  }),

  http.post("/api/tasks/:id/start", ({ params }) => {
    return transitionTo(params.id, "doing");
  }),

  http.post("/api/tasks/:id/complete", ({ params }) => {
    return transitionTo(params.id, "done");
  }),
];
