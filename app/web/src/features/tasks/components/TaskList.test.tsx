import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { server } from "../../../test/msw-server";
import { renderWithQueryClient } from "../../../test/renderWithQueryClient";
import type { TaskStatus } from "../domain/task";
import { TaskList } from "./TaskList";

// TaskList reads the active status filter via useSearch({ from: "/" }),
// which requires a full router context to resolve for real. Mocking
// the hook lets each test drive the filter directly, and mocking Link
// (used by the nested TaskItem) avoids needing that router context too.
const { mockUseSearch } = vi.hoisted(() => {
  return { mockUseSearch: vi.fn<() => { status: TaskStatus | "all" }>() };
});

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    useSearch: mockUseSearch,
    Link: ({ children, className }: { children: ReactNode; className?: string }) => (
      // `href` is required for jsdom/testing-library to expose the
      // implicit ARIA "link" role on a bare <a>.
      <a href="/tasks/mock" className={className}>
        {children}
      </a>
    ),
  };
});

describe("TaskList", () => {
  it('renders every task from the API, sorted by priority, for status "all" (normal)', async () => {
    mockUseSearch.mockReturnValue({ status: "all" });
    renderWithQueryClient(<TaskList />);

    await screen.findByText("Set up project scaffolding");

    const titles = screen.getAllByRole("link").map((link) => link.textContent);
    expect(titles).toEqual([
      "Set up project scaffolding",
      "Design the task domain model",
      "Review pull requests",
      "Write onboarding docs",
    ]);
  });

  it("filters the list down to only the requested status", async () => {
    mockUseSearch.mockReturnValue({ status: "todo" });
    renderWithQueryClient(<TaskList />);

    await screen.findByText("Review pull requests");

    const titles = screen.getAllByRole("link").map((link) => link.textContent);
    expect(titles).toEqual(["Review pull requests", "Write onboarding docs"]);
  });

  it("shows an empty state when no task matches the filter (boundary: zero results)", async () => {
    server.use(http.get("/api/tasks", () => HttpResponse.json([])));
    mockUseSearch.mockReturnValue({ status: "done" });
    renderWithQueryClient(<TaskList />);

    expect(await screen.findByText("No tasks found.")).toBeInTheDocument();
  });

  it("shows an error message when the request fails (abnormal)", async () => {
    server.use(
      http.get("/api/tasks", () => HttpResponse.json({ message: "boom" }, { status: 500 })),
    );
    mockUseSearch.mockReturnValue({ status: "all" });
    renderWithQueryClient(<TaskList />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Failed to load tasks");
  });
});
