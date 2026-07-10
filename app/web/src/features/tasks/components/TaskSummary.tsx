import { useSearch } from "@tanstack/react-router";
import { summarize } from "../domain/task";
import { useTasksQuery } from "../hooks/useTasks";

/**
 * Displays counts per status, derived via the domain `summarize`
 * function.
 *
 * Note (SPEC-008 R-5, known limitation): reads the same `limit`/`offset`
 * page as `TaskList` (so the two share one cached query instead of
 * firing a duplicate request), which means these counts describe only
 * the *current page*, not every task. A true whole-collection summary
 * would need a dedicated aggregate endpoint; out of scope for this Spec.
 */
export function TaskSummary() {
  const { limit, offset } = useSearch({ from: "/" });
  const { data } = useTasksQuery({ limit, offset });
  const summary = summarize(data?.items ?? []);

  return (
    <dl className="grid grid-cols-3 gap-2 text-center">
      <div className="rounded border border-gray-200 bg-white p-3">
        <dt className="text-xs uppercase text-gray-500">Todo</dt>
        <dd className="text-xl font-semibold">{summary.todo}</dd>
      </div>
      <div className="rounded border border-gray-200 bg-white p-3">
        <dt className="text-xs uppercase text-gray-500">Doing</dt>
        <dd className="text-xl font-semibold">{summary.doing}</dd>
      </div>
      <div className="rounded border border-gray-200 bg-white p-3">
        <dt className="text-xs uppercase text-gray-500">Done</dt>
        <dd className="text-xl font-semibold">{summary.done}</dd>
      </div>
    </dl>
  );
}
