import type { TaskStatus } from "../domain/task";

const STATUS_LABELS: Record<TaskStatus, string> = {
  todo: "Todo",
  doing: "Doing",
  done: "Done",
};

type TaskEmptyStateProps = {
  readonly status: TaskStatus | "all";
  readonly total: number;
};

/** Context-aware empty state for the task list. */
export function TaskEmptyState({ status, total }: TaskEmptyStateProps) {
  if (total === 0 && status === "all") {
    return (
      <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-gray-300 bg-surface-muted px-4 py-10 text-center">
        <p className="text-base font-medium text-gray-900">No tasks yet</p>
        <p className="max-w-sm text-sm text-gray-500">
          Create your first task using the form above to get started.
        </p>
      </div>
    );
  }

  const filterLabel = status === "all" ? "matching" : STATUS_LABELS[status];

  return (
    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-gray-300 bg-surface-muted px-4 py-10 text-center">
      <p className="text-base font-medium text-gray-900">No tasks found</p>
      <p className="max-w-sm text-sm text-gray-500">
        {status === "all"
          ? "No tasks on this page. Try another page or adjust your filters."
          : `No ${filterLabel} tasks on this page. Try another filter or page.`}
      </p>
    </div>
  );
}
