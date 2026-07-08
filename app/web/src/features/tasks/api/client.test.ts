import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "../../../test/msw-server";
import { ApiError } from "../../../shared/api/errors";
import type { Task } from "../domain/task";
import { fetchTaskById, updateTaskStatus } from "./client";
import type { TaskDto } from "./schema";

const validDto: TaskDto = {
  id: "1",
  title: "Set up project scaffolding",
  status: "done",
  priority: "high",
  created_at: "2026-07-01T09:00:00.000Z",
  updated_at: "2026-07-01T09:30:00.000Z",
};

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
    server.use(
      http.get("/api/tasks/:id", () =>
        HttpResponse.json({ message: "Task not found" }, { status: 404 }),
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

describe("updateTaskStatus", () => {
  it("encodeURIComponent-encodes special characters in the id into the request path (boundary)", async () => {
    let capturedPath = "";
    server.use(
      http.patch("/api/tasks/:id/status", ({ request }) => {
        capturedPath = new URL(request.url).pathname;
        return HttpResponse.json({ ...validDto, status: "doing" });
      }),
    );

    await updateTaskStatus("a/b c", "doing");

    expect(capturedPath).toBe(`/api/tasks/${encodeURIComponent("a/b c")}/status`);
    expect(capturedPath).toBe("/api/tasks/a%2Fb%20c/status");
  });
});
