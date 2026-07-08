import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { createTestQueryClient } from "../test/renderWithQueryClient";
import { router } from "./router";

/**
 * Full router-level integration test for the `/tasks/$taskId` route.
 *
 * `router` is the only export of router.tsx (TaskDetailPage,
 * taskDetailRoute and taskDetailParamsSchema are all module-private),
 * so the only way to exercise TaskDetailPage and its params validation
 * without changing production code is to drive the real, exported
 * `router` end to end with a memory history, the same way main.tsx
 * wires it up in the actual app (see also the "testability" note in
 * this file's final describe block).
 */
function renderRouterAt(initialPath: string) {
  const queryClient = createTestQueryClient();
  router.update({
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  });
  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    ),
  };
}

describe("/tasks/$taskId (TaskDetailPage, via the real router)", () => {
  it("shows a loading placeholder before the task resolves, then the task detail (normal)", async () => {
    renderRouterAt("/tasks/1");

    // useTaskQuery's fetch is asynchronous even against MSW, so the
    // very first render is still in its isLoading state.
    expect(await screen.findByText("Loading task...")).toBeInTheDocument();

    expect(
      await screen.findByRole("heading", { name: "Set up project scaffolding" }),
    ).toBeInTheDocument();
    expect(screen.getByText("status: done")).toBeInTheDocument();
    expect(screen.getByText("priority: high")).toBeInTheDocument();
  });

  it("shows an error message when the task does not exist (abnormal: 404)", async () => {
    renderRouterAt("/tasks/does-not-exist");

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Failed to load task: Task not found");
  });

  it("does not crash for a path with an empty taskId segment (boundary)", async () => {
    // A literal "//" produces an empty dynamic segment. TanStack
    // Router's own path matcher (not taskDetailParamsSchema) already
    // requires a non-empty segment for `$taskId`, so this never
    // reaches `/tasks/$taskId` at all: it falls through to the
    // router's default not-found handling instead of ever calling
    // `taskDetailParamsSchema.parse`. See the testability note below
    // for why `.min(1)` itself could not be exercised directly.
    renderRouterAt("/tasks//");

    expect(await screen.findByText("Not Found")).toBeInTheDocument();
  });
});

// Testability note (not a test, just documentation for the report):
// `taskDetailParamsSchema` (src/app/router.tsx) is not exported, so it
// cannot be imported and unit-tested in isolation without a production
// code change. A router-level attempt to reach an *empty* $taskId
// param was also tried and did not work: TanStack Router's own route
// matcher requires a non-empty path segment for a dynamic param before
// `params.parse` ever runs, so navigating to "/tasks//" (asserted above
// as a boundary case) or calling `router.navigate({ to:
// "/tasks/$taskId", params: { taskId: "" } })` both short-circuit
// before the schema is invoked (404 / a route-generation warning,
// respectively). This is reported as a testability gap rather than
// worked around.
