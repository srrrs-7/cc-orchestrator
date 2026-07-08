import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { server } from "../../../test/msw-server";
import { renderWithQueryClient } from "../../../test/renderWithQueryClient";
import type { Task, TaskStatus } from "../domain/task";
import { TaskItem } from "./TaskItem";

// TaskItem renders a router <Link> purely for navigation to the task
// detail page; that behavior belongs to the router, not this
// component. Replacing it with a plain anchor lets this test focus on
// TaskItem's own responsibility (button enablement + mutation calls)
// without standing up a full router context.
vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    Link: ({ children, className }: { children: ReactNode; className?: string }) => (
      // `href` is required for jsdom/testing-library to expose the
      // implicit ARIA "link" role on a bare <a>.
      <a href="/tasks/mock" className={className}>
        {children}
      </a>
    ),
  };
});

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: "42",
    title: "Write tests",
    status: "todo",
    priority: "medium",
    createdAt: "2026-01-01T00:00:00.000Z",
    updatedAt: "2026-01-01T00:00:00.000Z",
    ...overrides,
  };
}

describe("TaskItem", () => {
  it.each([
    { status: "todo" as TaskStatus, startEnabled: true, completeEnabled: false },
    { status: "doing" as TaskStatus, startEnabled: false, completeEnabled: true },
    { status: "done" as TaskStatus, startEnabled: false, completeEnabled: false },
  ])("enables Start=$startEnabled / Complete=$completeEnabled when status is $status", ({
    status,
    startEnabled,
    completeEnabled,
  }) => {
    renderWithQueryClient(<TaskItem task={makeTask({ status })} />);

    const startButton = screen.getByRole("button", { name: "Start" });
    const completeButton = screen.getByRole("button", { name: "Complete" });

    if (startEnabled) {
      expect(startButton).toBeEnabled();
    } else {
      expect(startButton).toBeDisabled();
    }

    if (completeEnabled) {
      expect(completeButton).toBeEnabled();
    } else {
      expect(completeButton).toBeDisabled();
    }
  });

  it("calls the update-status mutation with the next status when Start is clicked", async () => {
    const user = userEvent.setup();
    const received: { id?: string; status?: string } = {};
    server.use(
      http.patch("/api/tasks/:id/status", async ({ request, params }) => {
        const body = (await request.json()) as { status: string };
        received.id = String(params.id);
        received.status = body.status;
        return HttpResponse.json({
          id: String(params.id),
          title: "Write tests",
          status: body.status,
          priority: "medium",
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        });
      }),
    );

    renderWithQueryClient(<TaskItem task={makeTask({ status: "todo" })} />);
    await user.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => {
      expect(received).toEqual({ id: "42", status: "doing" });
    });
  });

  it("calls the update-status mutation with the next status when Complete is clicked", async () => {
    const user = userEvent.setup();
    const received: { id?: string; status?: string } = {};
    server.use(
      http.patch("/api/tasks/:id/status", async ({ request, params }) => {
        const body = (await request.json()) as { status: string };
        received.id = String(params.id);
        received.status = body.status;
        return HttpResponse.json({
          id: String(params.id),
          title: "Write tests",
          status: body.status,
          priority: "medium",
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        });
      }),
    );

    renderWithQueryClient(<TaskItem task={makeTask({ status: "doing" })} />);
    await user.click(screen.getByRole("button", { name: "Complete" }));

    await waitFor(() => {
      expect(received).toEqual({ id: "42", status: "done" });
    });
  });

  it("does not call the mutation when clicking a disabled button (abnormal: no-op)", async () => {
    const user = userEvent.setup();
    let called = false;
    server.use(
      http.patch("/api/tasks/:id/status", () => {
        called = true;
        return HttpResponse.json({ message: "should not be called" }, { status: 409 });
      }),
    );

    renderWithQueryClient(<TaskItem task={makeTask({ status: "done" })} />);
    await user.click(screen.getByRole("button", { name: "Start" }));
    await user.click(screen.getByRole("button", { name: "Complete" }));

    expect(called).toBe(false);
  });
});
