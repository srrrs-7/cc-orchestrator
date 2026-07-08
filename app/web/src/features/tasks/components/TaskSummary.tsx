import { summarize } from "../domain/task";
import { useTasksQuery } from "../hooks/useTasks";

/** Displays counts per status, derived via the domain `summarize` function. */
export function TaskSummary() {
  const { data } = useTasksQuery();
  const summary = summarize(data ?? []);

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
