import { HttpResponse, http } from "msw";
import { z } from "zod";
import type { TaskDto } from "../features/tasks/api/schema";
import { DEFAULT_LIMIT, MAX_LIMIT } from "../features/tasks/domain/pagination";
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

/**
 * A `limit`/`offset` query param, parsed and validated the same way
 * app/api does (SPEC-008, app/api/domain/task/page.go,
 * app/api/route/task_handler.go): a missing value falls back to
 * `fallback`. A present-but-non-integer value, or one outside the
 * lower bound, is rejected outright (this is where the mock
 * previously diverged from the real API by silently clamping
 * instead). Only the upper bound (`limit > MAX_LIMIT`) is clamped
 * rather than rejected.
 */
type PagingParamResult =
  | { readonly ok: true; readonly value: number }
  | { readonly ok: false; readonly error: string };

function parsePagingParam(
  raw: string | null,
  paramName: "limit" | "offset",
  fallback: number,
  min: number,
  max: number,
): PagingParamResult {
  if (raw === null) {
    return { ok: true, value: fallback };
  }
  const parsed = Number(raw);
  if (!Number.isInteger(parsed)) {
    return { ok: false, error: `${paramName} must be an integer` };
  }
  if (parsed < min) {
    return { ok: false, error: `${paramName} must be at least ${min}` };
  }
  return { ok: true, value: Math.min(parsed, max) };
}

/** Stable `created_at, id` ascending order, matching app/api's ORDER BY. */
function sortedTasks(): TaskDto[] {
  return [...tasks].sort((a, b) => {
    const createdDiff = a.created_at.localeCompare(b.created_at);
    return createdDiff !== 0 ? createdDiff : a.id.localeCompare(b.id);
  });
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
  http.get("/api/tasks", ({ request }) => {
    const url = new URL(request.url);
    const limitResult = parsePagingParam(
      url.searchParams.get("limit"),
      "limit",
      DEFAULT_LIMIT,
      1,
      MAX_LIMIT,
    );
    if (!limitResult.ok) {
      return HttpResponse.json({ error: limitResult.error }, { status: 400 });
    }
    const offsetResult = parsePagingParam(
      url.searchParams.get("offset"),
      "offset",
      0,
      0,
      Number.MAX_SAFE_INTEGER,
    );
    if (!offsetResult.ok) {
      return HttpResponse.json({ error: offsetResult.error }, { status: 400 });
    }

    const limit = limitResult.value;
    const offset = offsetResult.value;
    const all = sortedTasks();
    const items = all.slice(offset, offset + limit);
    return HttpResponse.json({ items, total: all.length, limit, offset });
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
