import { ZodError } from "zod";
import type { z } from "zod";
import { ensureValidAccessToken, refreshStoredSession } from "../../auth/domain/refresh";
import { clearAllAuthStorage } from "../../auth/domain/session";
import { ApiError } from "../../../shared/api/errors";
import type { Task } from "../domain/task";
import {
  getTasks,
  getTasksById,
  postTasks,
  postTasksByIdComplete,
  postTasksByIdStart,
} from "./generated";
import { client } from "./generated/client.gen";
import type { CreateTaskInput, TaskPage } from "./schema";
import { taskListSchema, taskSchema, toDomain, toDomainPage } from "./schema";

const rawBaseUrl = import.meta.env.VITE_API_BASE_URL;
const DEFAULT_BASE_PATH = "/api";

/**
 * The generated client builds a `Request` object internally (see
 * generated/client/client.gen.ts), which the Fetch spec resolves
 * relative URLs against `document.baseURI` -- real browsers do this,
 * but the Node-based test runner (MSW's Node interceptor patches the
 * global `Request` constructor and requires an absolute URL there) does
 * not. `window.location.origin` is defined both in production (a real
 * browser) and in tests (`environment: "jsdom"` in vitest.config.ts),
 * so resolving to an absolute URL here keeps behavior identical in
 * both environments instead of relying on relative-URL resolution.
 */
function resolveBaseUrl(): string {
  if (typeof rawBaseUrl === "string" && rawBaseUrl.length > 0) {
    return rawBaseUrl;
  }
  return `${window.location.origin}${DEFAULT_BASE_PATH}`;
}

client.setConfig({ baseUrl: resolveBaseUrl() });

// Inject the Bearer token on every outgoing request when a valid session exists.
// Attempts silent refresh when the access token is expired or near expiry.
client.interceptors.request.use(async (request: Request) => {
  const token = await ensureValidAccessToken();
  if (token === null) return request;
  const headers = new Headers(request.headers);
  headers.set("Authorization", `Bearer ${token}`);
  return new Request(request, { headers });
});

function hasErrorMessage(value: object): value is { error: unknown } {
  return "error" in value;
}

/** Reads the `{"error": string}` envelope (route.errorResponse) off a thrown value. */
function messageFrom(error: unknown): string | undefined {
  if (typeof error === "object" && error !== null && hasErrorMessage(error)) {
    return typeof error.error === "string" ? error.error : undefined;
  }
  return typeof error === "string" ? error : undefined;
}

/**
 * Normalizes every thrown value from the generated client (a parsed
 * `{error}` body, plain text, or a fetch-level network exception) into
 * an `ApiError`, so callers only ever see one error shape regardless of
 * which SDK function threw. This is the wire error boundary for the
 * whole feature (the standalone `shared/api/http.ts` fetch wrapper this
 * once mirrored has since been removed as dead code; see `parseResponse`
 * below for the equivalent boundary around successful-response parsing).
 */
client.interceptors.error.use(async (error, response) => {
  // On 401, attempt one silent refresh before forcing re-login.
  if (response?.status === 401) {
    const refreshed = await refreshStoredSession();
    if (refreshed === null) {
      clearAllAuthStorage();
      window.location.href = "/login";
    }
  }
  return new ApiError(messageFrom(error) ?? `Request failed with status ${response?.status ?? 0}`, {
    status: response?.status ?? 0,
    cause: error,
  });
});

/**
 * Validates an already-2xx response body against a generated Zod
 * schema and normalizes a schema mismatch into an `ApiError` instead of
 * letting a raw `ZodError` escape the wire error boundary (Minor-1: a
 * response that doesn't match this build's OpenAPI contract is still a
 * failure talking to the API, so callers -- and the UI, which reads
 * `error.message` -- should see the same `ApiError` shape as an HTTP
 * error, not internal Zod schema details).
 */
function parseResponse<T>(schema: z.ZodType<T>, data: unknown, status: number): T {
  try {
    return schema.parse(data);
  } catch (cause) {
    if (cause instanceof ZodError) {
      throw new ApiError("Received an unexpected response shape from the server", {
        status,
        cause,
      });
    }
    throw cause;
  }
}

export type FetchTasksParams = {
  readonly limit?: number;
  readonly offset?: number;
};

/**
 * GET /tasks — a page of tasks (SPEC-008), validated and mapped to the
 * domain `TaskPage` envelope. `limit`/`offset` are optional on the
 * request (the server applies its own defaults/clamping, see
 * app/api/domain/task/page.go), but the response always echoes back
 * the values it actually applied.
 */
export async function fetchTasks(params: FetchTasksParams = {}): Promise<TaskPage> {
  const { data, response } = await getTasks({
    query: { limit: params.limit, offset: params.offset },
    throwOnError: true,
  });
  return toDomainPage(parseResponse(taskListSchema, data, response.status));
}

/** GET /tasks/:id — a single task, validated and mapped to domain Task. */
export async function fetchTaskById(id: string): Promise<Task> {
  const { data, response } = await getTasksById({ path: { id }, throwOnError: true });
  return toDomain(parseResponse(taskSchema, data, response.status));
}

/** POST /tasks — create a task, validated and mapped to domain Task. */
export async function createTask(input: CreateTaskInput): Promise<Task> {
  const { data, response } = await postTasks({ body: input, throwOnError: true });
  return toDomain(parseResponse(taskSchema, data, response.status));
}

/**
 * POST /tasks/:id/start — transition a task from todo to doing.
 * (D2: replaces the old `PATCH /tasks/:id/status` call; Go's wire
 * contract has no PATCH .../status endpoint.)
 */
export async function startTask(id: string): Promise<Task> {
  const { data, response } = await postTasksByIdStart({ path: { id }, throwOnError: true });
  return toDomain(parseResponse(taskSchema, data, response.status));
}

/** POST /tasks/:id/complete — transition a task from doing to done. (D2, see startTask.) */
export async function completeTask(id: string): Promise<Task> {
  const { data, response } = await postTasksByIdComplete({ path: { id }, throwOnError: true });
  return toDomain(parseResponse(taskSchema, data, response.status));
}
