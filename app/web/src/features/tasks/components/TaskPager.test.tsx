import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { PageInfo } from "../domain/pagination";
import { TaskPager } from "./TaskPager";

// TaskPager reads/writes the `offset` URL search param via
// useNavigate({from: "/"}) (SPEC-008 R6). Mocking useNavigate lets each
// test both render the component without a full router context (same
// approach as TaskList.test.tsx's useSearch mock) and inspect exactly
// what search-updater function a click produces.
const { mockNavigate } = vi.hoisted(() => {
  return { mockNavigate: vi.fn() };
});

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

/** Extracts the `offset` produced by the `search` updater passed to the last `navigate` call. */
function lastNavigatedOffset(previousSearch: Record<string, unknown>): unknown {
  const lastCall = mockNavigate.mock.calls.at(-1);
  const options = lastCall?.[0] as { search: (prev: Record<string, unknown>) => object };
  return (options.search(previousSearch) as { offset: unknown }).offset;
}

describe("TaskPager", () => {
  it("disables the Previous button on the first page (boundary: offset=0)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    render(<TaskPager page={page} />);

    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).toBeEnabled();
  });

  it("enables the Previous button once past the first page (normal)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 20 };
    render(<TaskPager page={page} />);

    expect(screen.getByRole("button", { name: "Previous" })).toBeEnabled();
  });

  it("disables the Next button on the last (partial) page (boundary: remainder page)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 40 };
    render(<TaskPager page={page} />);

    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Previous" })).toBeEnabled();
  });

  it("disables the Next button when total exactly fills whole pages (boundary: exact multiple)", () => {
    const page: PageInfo = { total: 40, limit: 20, offset: 20 };
    render(<TaskPager page={page} />);

    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  });

  it("renders nothing when there are zero results (boundary: empty result set)", () => {
    const page: PageInfo = { total: 0, limit: 20, offset: 0 };
    const { container } = render(<TaskPager page={page} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("shows the 1-based displayed range and total", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 20 };
    render(<TaskPager page={page} />);

    expect(screen.getByText("21-40 of 57")).toBeInTheDocument();
  });

  it("navigates to the previous page's offset when Previous is clicked (normal)", async () => {
    mockNavigate.mockClear();
    const user = userEvent.setup();
    const page: PageInfo = { total: 57, limit: 20, offset: 40 };
    render(<TaskPager page={page} />);

    await user.click(screen.getByRole("button", { name: "Previous" }));

    expect(mockNavigate).toHaveBeenCalledTimes(1);
    expect(lastNavigatedOffset({ status: "all", limit: 20, offset: 40 })).toBe(20);
  });

  it("navigates to the next page's offset when Next is clicked (normal)", async () => {
    mockNavigate.mockClear();
    const user = userEvent.setup();
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    render(<TaskPager page={page} />);

    await user.click(screen.getByRole("button", { name: "Next" }));

    expect(mockNavigate).toHaveBeenCalledTimes(1);
    expect(lastNavigatedOffset({ status: "all", limit: 20, offset: 0 })).toBe(20);
  });

  it("clamps to offset 0 when Previous is clicked less than one page from the start (boundary)", async () => {
    mockNavigate.mockClear();
    const user = userEvent.setup();
    const page: PageInfo = { total: 30, limit: 20, offset: 10 };
    render(<TaskPager page={page} />);

    await user.click(screen.getByRole("button", { name: "Previous" }));

    expect(lastNavigatedOffset({ status: "all", limit: 20, offset: 10 })).toBe(0);
  });

  it("preserves other search params (e.g. status) when navigating (normal)", async () => {
    mockNavigate.mockClear();
    const user = userEvent.setup();
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    render(<TaskPager page={page} />);

    await user.click(screen.getByRole("button", { name: "Next" }));

    const lastCall = mockNavigate.mock.calls.at(-1);
    const options = lastCall?.[0] as { search: (prev: Record<string, unknown>) => object };
    const result = options.search({ status: "todo", limit: 20, offset: 0 });
    expect(result).toEqual({ status: "todo", limit: 20, offset: 20 });
  });
});
