import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "../../../test/msw-server";
import { createTestQueryClient } from "../../../test/renderWithQueryClient";
import type { TaskStatus } from "../domain/task";
import { useCompleteTask, useStartTask, useTaskQuery } from "./useTasks";

/**
 * Builds a wrapper that puts every hook rendered with it on the same
 * QueryClient instance, so cache invalidation triggered by one hook
 * (e.g. a mutation) is visible to another hook (e.g. a query) exactly
 * as it would be for two components in the same app tree.
 */
function wrapperFor(queryClient: ReturnType<typeof createTestQueryClient>) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe("useTaskQuery", () => {
  it("returns the task on success (normal)", async () => {
    const queryClient = createTestQueryClient();
    const { result } = renderHook(() => useTaskQuery("1"), { wrapper: wrapperFor(queryClient) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.title).toBe("Set up project scaffolding");
    expect(result.current.data?.status).toBe("done");
  });

  it("surfaces a 404 as an ApiError (abnormal)", async () => {
    const queryClient = createTestQueryClient();
    const { result } = renderHook(() => useTaskQuery("does-not-exist"), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.status).toBe(404);
    expect(result.current.error?.message).toBe("Task not found");
  });
});

// D2: `useUpdateTaskStatus` (PATCH /tasks/:id/status) no longer exists;
// it is replaced by `useStartTask`/`useCompleteTask` (POST
// /tasks/:id/start | /complete), each exercised below for the same
// invalidation behavior the old single hook was checked for.
describe("query invalidation after useStartTask", () => {
  it("refetches the ['tasks', id] detail query once the mutation invalidates ['tasks'] (integration)", async () => {
    let status: TaskStatus = "todo";
    server.use(
      http.get("/api/tasks/:id", ({ params }) => {
        if (params.id !== "99") {
          return HttpResponse.json({ error: "Task not found" }, { status: 404 });
        }
        return HttpResponse.json({
          id: "99",
          title: "Invalidation target",
          status,
          priority: "medium",
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        });
      }),
      http.post("/api/tasks/:id/start", ({ params }) => {
        if (params.id !== "99") {
          return HttpResponse.json({ error: "Task not found" }, { status: 404 });
        }
        status = "doing";
        return HttpResponse.json({
          id: "99",
          title: "Invalidation target",
          status,
          priority: "medium",
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        });
      }),
    );

    const queryClient = createTestQueryClient();
    const wrapper = wrapperFor(queryClient);

    const detail = renderHook(() => useTaskQuery("99"), { wrapper });
    await waitFor(() => expect(detail.result.current.isSuccess).toBe(true));
    expect(detail.result.current.data?.status).toBe("todo");

    const mutation = renderHook(() => useStartTask(), { wrapper });
    await act(async () => {
      await mutation.result.current.mutateAsync("99");
    });

    // The mutation's onSuccess invalidates the ["tasks"] query key, which
    // is a prefix of the detail query's ["tasks", "99"] key. If that
    // invalidation reaches the detail query, it refetches and the hook's
    // data reflects the new status without any manual refetch call here.
    await waitFor(() => expect(detail.result.current.data?.status).toBe("doing"));
  });
});

describe("query invalidation after useCompleteTask", () => {
  it("refetches the ['tasks', id] detail query once the mutation invalidates ['tasks'] (integration)", async () => {
    let status: TaskStatus = "doing";
    server.use(
      http.get("/api/tasks/:id", ({ params }) => {
        if (params.id !== "100") {
          return HttpResponse.json({ error: "Task not found" }, { status: 404 });
        }
        return HttpResponse.json({
          id: "100",
          title: "Invalidation target",
          status,
          priority: "medium",
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        });
      }),
      http.post("/api/tasks/:id/complete", ({ params }) => {
        if (params.id !== "100") {
          return HttpResponse.json({ error: "Task not found" }, { status: 404 });
        }
        status = "done";
        return HttpResponse.json({
          id: "100",
          title: "Invalidation target",
          status,
          priority: "medium",
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        });
      }),
    );

    const queryClient = createTestQueryClient();
    const wrapper = wrapperFor(queryClient);

    const detail = renderHook(() => useTaskQuery("100"), { wrapper });
    await waitFor(() => expect(detail.result.current.isSuccess).toBe(true));
    expect(detail.result.current.data?.status).toBe("doing");

    const mutation = renderHook(() => useCompleteTask(), { wrapper });
    await act(async () => {
      await mutation.result.current.mutateAsync("100");
    });

    await waitFor(() => expect(detail.result.current.data?.status).toBe("done"));
  });
});
