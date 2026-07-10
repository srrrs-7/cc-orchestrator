import { describe, expect, it } from "vitest";
import { InvalidTransitionError } from "./errors";
import type { Task } from "./task";
import {
  canComplete,
  canStart,
  completeTask,
  filterByStatus,
  sortByPriority,
  startTask,
  summarize,
} from "./task";

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: "1",
    title: "Sample task",
    status: "todo",
    priority: "medium",
    createdAt: "2026-01-01T00:00:00.000Z",
    updatedAt: "2026-01-01T00:00:00.000Z",
    ...overrides,
  };
}

describe("startTask", () => {
  it("moves a todo task to doing (normal)", () => {
    const task = makeTask({ status: "todo" });

    const result = startTask(task);

    expect(result.status).toBe("doing");
    expect(result).not.toBe(task);
    // Immutability: the input task itself must not be mutated.
    expect(task.status).toBe("todo");
  });

  it.each([
    ["doing"],
    ["done"],
  ] as const)("throws InvalidTransitionError with from/to when called from %s (abnormal)", (status) => {
    const task = makeTask({ status });

    expect(() => startTask(task)).toThrow(InvalidTransitionError);
    try {
      startTask(task);
      throw new Error("expected startTask to throw");
    } catch (error) {
      expect(error).toBeInstanceOf(InvalidTransitionError);
      const transitionError = error as InvalidTransitionError;
      expect(transitionError.from).toBe(status);
      expect(transitionError.to).toBe("doing");
    }
  });
});

describe("completeTask", () => {
  it("moves a doing task to done (normal)", () => {
    const task = makeTask({ status: "doing" });

    const result = completeTask(task);

    expect(result.status).toBe("done");
    expect(result).not.toBe(task);
    expect(task.status).toBe("doing");
  });

  it.each([
    ["todo"],
    ["done"],
  ] as const)("throws InvalidTransitionError with from/to when called from %s (abnormal)", (status) => {
    const task = makeTask({ status });

    try {
      completeTask(task);
      throw new Error("expected completeTask to throw");
    } catch (error) {
      expect(error).toBeInstanceOf(InvalidTransitionError);
      const transitionError = error as InvalidTransitionError;
      expect(transitionError.from).toBe(status);
      expect(transitionError.to).toBe("done");
    }
  });
});

describe("canStart", () => {
  it.each([
    { status: "todo", expected: true },
    { status: "doing", expected: false },
    { status: "done", expected: false },
  ] as const)("returns $expected for status $status (boundary: every status)", ({
    status,
    expected,
  }) => {
    expect(canStart(makeTask({ status }))).toBe(expected);
  });
});

describe("canComplete", () => {
  it.each([
    { status: "todo", expected: false },
    { status: "doing", expected: true },
    { status: "done", expected: false },
  ] as const)("returns $expected for status $status (boundary: every status)", ({
    status,
    expected,
  }) => {
    expect(canComplete(makeTask({ status }))).toBe(expected);
  });
});

describe("filterByStatus", () => {
  const tasks: Task[] = [
    makeTask({ id: "1", status: "todo" }),
    makeTask({ id: "2", status: "doing" }),
    makeTask({ id: "3", status: "done" }),
    makeTask({ id: "4", status: "todo" }),
  ];

  it.each([
    "todo",
    "doing",
    "done",
  ] as const)("returns only tasks with status %s (normal)", (status) => {
    const result = filterByStatus(tasks, status);
    expect(result.every((task) => task.status === status)).toBe(true);
    expect(result).toEqual(tasks.filter((task) => task.status === status));
  });

  it('returns every task, unfiltered, for status "all"', () => {
    const result = filterByStatus(tasks, "all");
    expect(result).toEqual(tasks);
    expect(result).not.toBe(tasks);
  });

  it("returns an empty array when no task matches the status (boundary)", () => {
    const onlyTodo: Task[] = [makeTask({ id: "1", status: "todo" })];
    expect(filterByStatus(onlyTodo, "done")).toEqual([]);
  });

  it("does not mutate the input array", () => {
    const before = [...tasks];
    filterByStatus(tasks, "todo");
    filterByStatus(tasks, "all");
    expect(tasks).toEqual(before);
  });
});

describe("sortByPriority", () => {
  it("orders tasks high -> medium -> low (normal)", () => {
    const tasks: Task[] = [
      makeTask({ id: "low", priority: "low", createdAt: "2026-01-03T00:00:00.000Z" }),
      makeTask({ id: "high", priority: "high", createdAt: "2026-01-01T00:00:00.000Z" }),
      makeTask({ id: "medium", priority: "medium", createdAt: "2026-01-02T00:00:00.000Z" }),
    ];

    const result = sortByPriority(tasks);

    expect(result.map((task) => task.id)).toEqual(["high", "medium", "low"]);
  });

  it("breaks same-priority ties by createdAt ascending, preserving relative order for exact ties (stability)", () => {
    const tasks: Task[] = [
      makeTask({ id: "a", priority: "high", createdAt: "2026-01-01T00:00:00.000Z" }),
      makeTask({ id: "b", priority: "high", createdAt: "2026-01-01T00:00:00.000Z" }),
      makeTask({ id: "c", priority: "high", createdAt: "2025-12-31T00:00:00.000Z" }),
    ];

    const result = sortByPriority(tasks);

    // "c" has the earliest createdAt so it sorts first; "a" and "b" are
    // fully tied (same priority, same createdAt), so a stable sort must
    // preserve their original relative order (a before b).
    expect(result.map((task) => task.id)).toEqual(["c", "a", "b"]);
  });

  it("returns an empty array for an empty input (boundary)", () => {
    expect(sortByPriority([])).toEqual([]);
  });

  it("does not mutate the input array or its tasks", () => {
    const tasks: Task[] = [
      makeTask({ id: "1", priority: "low" }),
      makeTask({ id: "2", priority: "high" }),
    ];
    const before = tasks.map((task) => ({ ...task }));

    const result = sortByPriority(tasks);

    expect(tasks).toEqual(before);
    expect(result).not.toBe(tasks);
  });
});

describe("summarize", () => {
  it("counts tasks per status for a mixed list (normal)", () => {
    const tasks: Task[] = [
      makeTask({ status: "todo" }),
      makeTask({ status: "todo" }),
      makeTask({ status: "doing" }),
      makeTask({ status: "done" }),
    ];

    expect(summarize(tasks)).toEqual({ todo: 2, doing: 1, done: 1 });
  });

  it("returns all-zero counts for an empty list (boundary)", () => {
    expect(summarize([])).toEqual({ todo: 0, doing: 0, done: 0 });
  });

  it("does not mutate the input array or its tasks", () => {
    const tasks: Task[] = [makeTask({ status: "todo" }), makeTask({ status: "done" })];
    const before = tasks.map((task) => ({ ...task }));

    summarize(tasks);

    expect(tasks).toEqual(before);
  });
});
