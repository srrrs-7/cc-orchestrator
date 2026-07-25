import { Link } from "@tanstack/react-router";
import { Button } from "../../../shared/ui/Button";
import { Card } from "../../../shared/ui/Card";
import { formatDateTime } from "../domain/format";
import type { Task } from "../domain/task";
import { canComplete, canStart } from "../domain/task";
import { useCompleteTask, useStartTask } from "../hooks/useTasks";
import { PriorityBadge } from "./PriorityBadge";
import { StatusBadge } from "./StatusBadge";

type TaskItemProps = {
  readonly task: Task;
  readonly showTimestamps?: boolean;
};

/**
 * Renders a single task. Holds no business logic itself: whether the
 * Start/Complete buttons are enabled comes from domain predicates
 * (canStart/canComplete). Which endpoint to call (POST .../start or
 * .../complete, D2) is implicit in which button was clicked, so no
 * domain transition function is needed here to compute a "next status"
 * to send.
 */
export function TaskItem({ task, showTimestamps = false }: TaskItemProps) {
  const startTask = useStartTask();
  const completeTask = useCompleteTask();

  const handleStart = () => {
    if (!canStart(task)) return;
    startTask.mutate(task.id);
  };

  const handleComplete = () => {
    if (!canComplete(task)) return;
    completeTask.mutate(task.id);
  };

  const isMutating = startTask.isPending || completeTask.isPending;

  return (
    <Card className="flex flex-col gap-3 p-4 transition-shadow motion-reduce:transition-none hover:shadow-md sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <div className="flex min-w-0 flex-col gap-2">
        <Link
          to="/tasks/$taskId"
          params={{ taskId: task.id }}
          className="break-words text-base font-medium text-gray-900 hover:text-accent focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
        >
          {task.title}
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge status={task.status} />
          <PriorityBadge priority={task.priority} />
        </div>
        {showTimestamps ? (
          <dl className="flex flex-col gap-1 text-xs text-gray-500 sm:flex-row sm:flex-wrap sm:gap-x-4">
            <div className="min-w-0">
              <dt className="sr-only">Created</dt>
              <dd>
                Created <time dateTime={task.createdAt}>{formatDateTime(task.createdAt)}</time>
              </dd>
            </div>
            <div className="min-w-0">
              <dt className="sr-only">Updated</dt>
              <dd>
                Updated <time dateTime={task.updatedAt}>{formatDateTime(task.updatedAt)}</time>
              </dd>
            </div>
          </dl>
        ) : null}
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">
        <Button variant="secondary" onClick={handleStart} disabled={!canStart(task) || isMutating}>
          {startTask.isPending ? "Starting…" : "Start"}
        </Button>
        <Button onClick={handleComplete} disabled={!canComplete(task) || isMutating}>
          {completeTask.isPending ? "Completing…" : "Complete"}
        </Button>
      </div>
    </Card>
  );
}
