import { useNavigate, useSearch } from "@tanstack/react-router";
import type { TaskStatus } from "../domain/task";
import { summarize } from "../domain/task";
import { useTasksQuery } from "../hooks/useTasks";

type SummaryCardProps = {
  readonly label: string;
  readonly count: number;
  readonly filterStatus: TaskStatus;
  readonly isActive: boolean;
  readonly accentClassName: string;
  readonly onSelect: (status: TaskStatus) => void;
};

function SummaryCard({
  label,
  count,
  filterStatus,
  isActive,
  accentClassName,
  onSelect,
}: SummaryCardProps) {
  return (
    <button
      type="button"
      aria-pressed={isActive}
      aria-label={`Show ${label} tasks (${count})`}
      onClick={() => onSelect(filterStatus)}
      className={`min-w-0 rounded-lg border bg-surface p-3 text-center shadow-sm transition-shadow motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 pointer-coarse:min-h-11 ${
        isActive
          ? `border-accent ring-2 ring-accent/20 ${accentClassName}`
          : "border-border-subtle hover:border-gray-300 hover:shadow-md"
      }`}
    >
      <span className="block text-xs font-medium uppercase tracking-wide text-gray-500">
        {label}
      </span>
      <span className="mt-1 block text-2xl font-semibold tabular-nums text-gray-900">{count}</span>
    </button>
  );
}

/**
 * Displays counts per status, derived via the domain `summarize`
 * function. Each card is clickable to filter the list by that status.
 *
 * Note (SPEC-008 R-5, known limitation): reads the same `limit`/`offset`
 * page as `TaskList` (so the two share one cached query instead of
 * firing a duplicate request), which means these counts describe only
 * the *current page*, not every task. A true whole-collection summary
 * would need a dedicated aggregate endpoint; out of scope for this Spec.
 */
export function TaskSummary() {
  const { status, limit, offset } = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });
  const { data } = useTasksQuery({ limit, offset });
  const summary = summarize(data?.items ?? []);

  const selectStatus = (nextStatus: TaskStatus) => {
    navigate({ search: (prev) => ({ ...prev, status: nextStatus, offset: 0 }) });
  };

  return (
    <section aria-labelledby="task-summary-heading">
      <h2 id="task-summary-heading" className="sr-only">
        Task summary
      </h2>
      <div className="grid grid-cols-3 gap-2 sm:gap-3">
        <SummaryCard
          label="Todo"
          count={summary.todo}
          filterStatus="todo"
          isActive={status === "todo"}
          accentClassName="bg-status-todo-bg"
          onSelect={selectStatus}
        />
        <SummaryCard
          label="Doing"
          count={summary.doing}
          filterStatus="doing"
          isActive={status === "doing"}
          accentClassName="bg-status-doing-bg"
          onSelect={selectStatus}
        />
        <SummaryCard
          label="Done"
          count={summary.done}
          filterStatus="done"
          isActive={status === "done"}
          accentClassName="bg-status-done-bg"
          onSelect={selectStatus}
        />
      </div>
    </section>
  );
}
