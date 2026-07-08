import { Link } from "@tanstack/react-router";
import { Button } from "../../../shared/ui/Button";
import type { Task } from "../domain/task";
import { canComplete, canStart, completeTask, startTask } from "../domain/task";
import { useUpdateTaskStatus } from "../hooks/useTasks";

type TaskItemProps = {
  readonly task: Task;
};

/**
 * Renders a single task. Holds no business logic itself: whether the
 * Start/Complete buttons are enabled comes from domain predicates
 * (canStart/canComplete), and the next status to persist comes from
 * the domain transition functions (startTask/completeTask).
 */
export function TaskItem({ task }: TaskItemProps) {
  const updateStatus = useUpdateTaskStatus();

  const handleStart = () => {
    if (!canStart(task)) return;
    const next = startTask(task);
    updateStatus.mutate({ id: task.id, status: next.status });
  };

  const handleComplete = () => {
    if (!canComplete(task)) return;
    const next = completeTask(task);
    updateStatus.mutate({ id: task.id, status: next.status });
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
          disabled={!canStart(task) || updateStatus.isPending}
        >
          Start
        </Button>
        <Button onClick={handleComplete} disabled={!canComplete(task) || updateStatus.isPending}>
          Complete
        </Button>
      </div>
    </div>
  );
}
