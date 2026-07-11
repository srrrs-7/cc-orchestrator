import { useNavigate, useSearch } from "@tanstack/react-router";
import type { TaskStatus } from "../domain/task";

type StatusOption = {
  readonly value: TaskStatus | "all";
  readonly label: string;
};

const STATUS_OPTIONS: readonly StatusOption[] = [
  { value: "all", label: "All" },
  { value: "todo", label: "Todo" },
  { value: "doing", label: "Doing" },
  { value: "done", label: "Done" },
];

/** Status filter, synced with the `status` URL search param on `/`. */
export function TaskFilters() {
  const { status } = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });

  return (
    <section aria-labelledby="task-filters-heading" className="flex flex-col gap-2">
      <h2 id="task-filters-heading" className="text-sm font-medium text-gray-700">
        Filter by status
      </h2>
      <fieldset className="flex flex-wrap gap-2 border-0 p-0">
        <legend className="sr-only">Task status filter</legend>
        {STATUS_OPTIONS.map((option) => {
          const isActive = status === option.value;
          return (
            <button
              key={option.value}
              type="button"
              aria-pressed={isActive}
              onClick={() =>
                navigate({ search: (prev) => ({ ...prev, status: option.value, offset: 0 }) })
              }
              className={
                isActive
                  ? "rounded-full bg-accent px-4 py-1.5 text-sm font-medium text-white shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 pointer-coarse:min-h-11"
                  : "rounded-full border border-border-subtle bg-surface px-4 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:border-gray-300 hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 pointer-coarse:min-h-11"
              }
            >
              {option.label}
            </button>
          );
        })}
      </fieldset>
    </section>
  );
}
