import { Link } from "@tanstack/react-router";
import { Button } from "../../../shared/ui/Button";
import type { Task } from "../domain/task";
import { canComplete, canStart } from "../domain/task";
import { useCompleteTask, useStartTask } from "../hooks/useTasks";

type TaskItemProps = {
  readonly task: Task;
};

/**
 * Renders a single task. Holds no business logic itself: whether the
 * Start/Complete buttons are enabled comes from domain predicates
 * (canStart/canComplete). Which endpoint to call (POST .../start or
 * .../complete, D2) is implicit in which button was clicked, so no
 * domain transition function is needed here to compute a "next status"
 * to send.
 */
export function TaskItem({ task }: TaskItemProps) {
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

  return (
    <div className="flex items-center justify-between gap-4 rounded border border-gray-200 bg-white p-3">
      <div className="flex flex-col gap-1">
        <Link
          to="/tasks/$taskId"
          params={{ taskId: task.id }}
          className="font-medium hover:underline"
        >
          {task.title}
        </Link>
        <div className="flex gap-2 text-xs text-gray-500">
          <span>status: {task.status}</span>
          <span>priority: {task.priority}</span>
        </div>
      </div>
      <div className="flex gap-2">
        <Button
          variant="secondary"
          onClick={handleStart}
          disabled={!canStart(task) || startTask.isPending}
        >
          Start
        </Button>
        <Button onClick={handleComplete} disabled={!canComplete(task) || completeTask.isPending}>
          Complete
        </Button>
      </div>
    </div>
  );
}
