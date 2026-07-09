import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { ZodError } from "zod";
import { server } from "../../../test/msw-server";
import { ApiError } from "../../../shared/api/errors";
import type { Task } from "../domain/task";
import { completeTask, createTask, fetchTaskById, fetchTasks, startTask } from "./client";
import type { TaskDto } from "./schema";

/** The generic message `parseResponse` throws for any schema-validation failure. */
const UNEXPECTED_SHAPE_MESSAGE = "Received an unexpected response shape from the server";

const validDto: TaskDto = {
  id: "1",
  title: "Set up project scaffolding",
  status: "done",
  priority: "high",
  created_at: "2026-07-01T09:00:00.000Z",
  updated_at: "2026-07-01T09:30:00.000Z",
};

// Minor-1: a 200 response that doesn't match the generated Zod schema
// (`zGetTasksResponse`/`zRouteTaskResponse`) must surface as an `ApiError`,
// not a raw `ZodError`, so the wire error boundary is uniform regardless of
// whether the failure came from HTTP or from response-shape validation.
describe("fetchTasks", () => {
  it("returns an empty list without throwing when the server responds with zero tasks (boundary)", async () => {
    server.use(http.get("/api/tasks", () => HttpResponse.json([])));

    const tasks = await fetchTasks();

    expect(tasks).toEqual([]);
  });

  it("throws an ApiError, not a raw ZodError, when a 200 response item is missing a required field (abnormal)", async () => {
    const { title, ...withoutTitle } = validDto;
    void title;
    server.use(http.get("/api/tasks", () => HttpResponse.json([withoutTitle])));

    let caught: unknown;
    try {
      await fetchTasks();
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).not.toBeInstanceOf(ZodError);
    expect((caught as ApiError).message).toBe(UNEXPECTED_SHAPE_MESSAGE);
  });

  it("throws an ApiError when one item in an otherwise-valid list has an out-of-enum status (abnormal: partially invalid list)", async () => {
    server.use(
      http.get("/api/tasks", () =>
        HttpResponse.json([validDto, { ...validDto, id: "2", status: "archived" }]),
      ),
    );

    let caught: unknown;
    try {
      await fetchTasks();
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).message).toBe(UNEXPECTED_SHAPE_MESSAGE);
  });
});

describe("fetchTaskById", () => {
  it("returns the domain Task mapped from the DTO (normal)", async () => {
    server.use(http.get("/api/tasks/:id", () => HttpResponse.json(validDto)));

    const task = await fetchTaskById("1");

    const expected: Task = {
      id: validDto.id,
      title: validDto.title,
      status: validDto.status,
      priority: validDto.priority,
      createdAt: validDto.created_at,
      updatedAt: validDto.updated_at,
    };
    expect(task).toEqual(expected);
  });

  it("throws an ApiError with status 404 when the task does not exist (abnormal)", async () => {
    // D3: the Go wire contract wraps errors as {"error": string}, not
    // {"message": string}.
    server.use(
      http.get("/api/tasks/:id", () =>
        HttpResponse.json({ error: "Task not found" }, { status: 404 }),
      ),
    );

    let caught: unknown;
    try {
      await fetchTaskById("does-not-exist");
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).status).toBe(404);
    expect((caught as ApiError).message).toBe("Task not found");
  });

  it("throws an ApiError, not a raw ZodError, when a 200 response field has the wrong type (abnormal: type-invalid field)", async () => {
    server.use(
      http.get("/api/tasks/:id", () => HttpResponse.json({ ...validDto, updated_at: 12345 })),
    );

    let caught: unknown;
    try {
      await fetchTaskById("1");
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).not.toBeInstanceOf(ZodError);
    expect((caught as ApiError).message).toBe(UNEXPECTED_SHAPE_MESSAGE);
  });

  it("throws an ApiError when the 200 response has an out-of-enum priority (abnormal)", async () => {
    server.use(
      http.get("/api/tasks/:id", () => HttpResponse.json({ ...validDto, priority: "urgent" })),
    );

    let caught: unknown;
    try {
      await fetchTaskById("1");
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).message).toBe(UNEXPECTED_SHAPE_MESSAGE);
  });

  it("encodeURIComponent-encodes special characters in the id into the request path (boundary)", async () => {
    let capturedPath = "";
    server.use(
      http.get("/api/tasks/:id", ({ request }) => {
        capturedPath = new URL(request.url).pathname;
        return HttpResponse.json(validDto);
      }),
    );

    await fetchTaskById("a/b c");

    expect(capturedPath).toBe(`/api/tasks/${encodeURIComponent("a/b c")}`);
    expect(capturedPath).toBe("/api/tasks/a%2Fb%20c");
  });
});

describe("createTask", () => {
  it("throws an ApiError, not a raw ZodError, when the 200 response is missing a required field (abnormal)", async () => {
    const { id, ...withoutId } = validDto;
    void id;
    server.use(http.post("/api/tasks", () => HttpResponse.json(withoutId, { status: 201 })));

    let caught: unknown;
    try {
      await createTask({ title: "Write more tests", priority: "low" });
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect(caught).not.toBeInstanceOf(ZodError);
    expect((caught as ApiError).message).toBe(UNEXPECTED_SHAPE_MESSAGE);
  });

  it("throws an ApiError when the 200 response has an out-of-enum status (abnormal)", async () => {
    server.use(
      http.post("/api/tasks", () =>
        HttpResponse.json({ ...validDto, status: "archived" }, { status: 201 }),
      ),
    );

    let caught: unknown;
    try {
      await createTask({ title: "Write more tests", priority: "low" });
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).message).toBe(UNEXPECTED_SHAPE_MESSAGE);
  });
});

// D2: `updateTaskStatus` (PATCH /tasks/:id/status) no longer exists on
// the Go wire contract; it is replaced by two dedicated transitions,
// each exercised below.
describe("startTask", () => {
  it("POSTs to /tasks/:id/start and returns the domain Task mapped from the response (normal)", async () => {
    let capturedMethod = "";
    server.use(
      http.post("/api/tasks/:id/start", ({ request }) => {
        capturedMethod = request.method;
        return HttpResponse.json({ ...validDto, status: "doing" });
      }),
    );

    const task = await startTask("1");

    expect(capturedMethod).toBe("POST");
    expect(task.status).toBe("doing");
  });

  it("encodeURIComponent-encodes special characters in the id into the request path (boundary)", async () => {
    let capturedPath = "";
    server.use(
      http.post("/api/tasks/:id/start", ({ request }) => {
        capturedPath = new URL(request.url).pathname;
        return HttpResponse.json({ ...validDto, status: "doing" });
      }),
    );

    await startTask("a/b c");

    expect(capturedPath).toBe(`/api/tasks/${encodeURIComponent("a/b c")}/start`);
    expect(capturedPath).toBe("/api/tasks/a%2Fb%20c/start");
  });
});

describe("completeTask", () => {
  it("POSTs to /tasks/:id/complete and returns the domain Task mapped from the response (normal)", async () => {
    let capturedMethod = "";
    server.use(
      http.post("/api/tasks/:id/complete", ({ request }) => {
        capturedMethod = request.method;
        return HttpResponse.json({ ...validDto, status: "done" });
      }),
    );

    const task = await completeTask("2");

    expect(capturedMethod).toBe("POST");
    expect(task.status).toBe("done");
  });

  it("encodeURIComponent-encodes special characters in the id into the request path (boundary)", async () => {
    let capturedPath = "";
    server.use(
      http.post("/api/tasks/:id/complete", ({ request }) => {
        capturedPath = new URL(request.url).pathname;
        return HttpResponse.json({ ...validDto, status: "done" });
      }),
    );

    await completeTask("a/b c");

    expect(capturedPath).toBe(`/api/tasks/${encodeURIComponent("a/b c")}/complete`);
    expect(capturedPath).toBe("/api/tasks/a%2Fb%20c/complete");
  });
});
