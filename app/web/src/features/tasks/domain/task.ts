import { InvalidTransitionError } from "./errors";

/**
 * Domain layer for the "tasks" feature.
 *
 * This module is deliberately dependency-free: no React, no fetch, no
 * DOM, no zod, no framework of any kind. It only exports plain types
 * and pure functions so that business rules (status transitions,
 * predicates, derived values) can be reasoned about and reused
 * independently of how the data was fetched or how it is rendered.
 *
 * Status names and the transition rule are mirrored from the Go API's
 * task aggregate (app/api/domain/task/status.go, task.go): a Task is
 * created as "todo", moves to "doing" via Start, and to "done" via
 * Complete. No other transition is valid.
 */

export const TASK_STATUSES = ["todo", "doing", "done"] as const;
export type TaskStatus = (typeof TASK_STATUSES)[number];

/**
 * Priority is not part of the app/api Task aggregate today; it is a
 * frontend-only enrichment used to demonstrate derived-value domain
 * functions (sortByPriority). See the implementation report for this
 * deviation.
 */
export const TASK_PRIORITIES = ["low", "medium", "high"] as const;
export type TaskPriority = (typeof TASK_PRIORITIES)[number];

export type Task = {
  readonly id: string;
  readonly title: string;
  readonly status: TaskStatus;
  readonly priority: TaskPriority;
  readonly createdAt: string;
  readonly updatedAt: string;
};

/**
 * Mirrors Status.CanTransitionTo in app/api/domain/task/status.go:
 * only todo -> doing and doing -> done are allowed.
 */
function canTransitionTo(from: TaskStatus, to: TaskStatus): boolean {
  if (from === "todo" && to === "doing") return true;
  if (from === "doing" && to === "done") return true;
  return false;
}

/** Predicate mirroring the Task.Start precondition. */
export function canStart(task: Task): boolean {
  return canTransitionTo(task.status, "doing");
}

/** Predicate mirroring the Task.Complete precondition. */
export function canComplete(task: Task): boolean {
  return canTransitionTo(task.status, "done");
}

/**
 * Mirrors Task.Start. Returns a new Task in the "doing" status.
 * Throws InvalidTransitionError if the current status does not allow
 * it (mirrors the *TransitionError returned by the Go aggregate).
 */
export function startTask(task: Task): Task {
  if (!canStart(task)) {
    throw new InvalidTransitionError(task.status, "doing");
  }
  return { ...task, status: "doing" };
}

/**
 * Mirrors Task.Complete. Returns a new Task in the "done" status.
 * Throws InvalidTransitionError if the current status does not allow
 * it.
 */
export function completeTask(task: Task): Task {
  if (!canComplete(task)) {
    throw new InvalidTransitionError(task.status, "done");
  }
  return { ...task, status: "done" };
}

/** Filters tasks by status. "all" returns every task, unfiltered. */
export function filterByStatus(tasks: readonly Task[], status: TaskStatus | "all"): Task[] {
  if (status === "all") {
    return [...tasks];
  }
  return tasks.filter((task) => task.status === status);
}

const PRIORITY_RANK: Record<TaskPriority, number> = {
  high: 0,
  medium: 1,
  low: 2,
};

/**
 * Returns a new array of tasks sorted by priority (high first), then
 * by creation date (oldest first) as a stable tie-breaker. Does not
 * mutate the input.
 */
export function sortByPriority(tasks: readonly Task[]): Task[] {
  return [...tasks].sort((a, b) => {
    const priorityDiff = PRIORITY_RANK[a.priority] - PRIORITY_RANK[b.priority];
    if (priorityDiff !== 0) return priorityDiff;
    return a.createdAt.localeCompare(b.createdAt);
  });
}

export type TaskStatusSummary = {
  readonly todo: number;
  readonly doing: number;
  readonly done: number;
};

/** Derives counts per status. Useful for a dashboard-style summary. */
export function summarize(tasks: readonly Task[]): TaskStatusSummary {
  const counts = { todo: 0, doing: 0, done: 0 };
  for (const task of tasks) {
    counts[task.status] += 1;
  }
  return counts;
}
