import type { TaskStatus } from "./task";

/**
 * Thrown when attempting to move a Task from one status to another
 * that violates the allowed state machine (see `Status.CanTransitionTo`
 * in app/api/domain/task/status.go, which this mirrors: todo -> doing
 * -> done only).
 *
 * This is a typed error (rather than a plain Error) so callers can
 * branch on it via `instanceof` and inspect the attempted transition.
 */
export class InvalidTransitionError extends Error {
  readonly from: TaskStatus;
  readonly to: TaskStatus;

  constructor(from: TaskStatus, to: TaskStatus) {
    super(`Cannot transition task from "${from}" to "${to}"`);
    this.name = "InvalidTransitionError";
    this.from = from;
    this.to = to;
  }
}
