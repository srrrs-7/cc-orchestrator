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
    <fieldset className="flex flex-wrap gap-2 border-0 p-0">
      <legend className="sr-only">Task status filter</legend>
      {STATUS_OPTIONS.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={status === option.value}
          onClick={() =>
            navigate({ search: (prev) => ({ ...prev, status: option.value, offset: 0 }) })
          }
          className={
            status === option.value
              ? "rounded bg-blue-600 px-3 py-1 text-sm text-white pointer-coarse:min-h-11 pointer-coarse:px-4"
              : "rounded bg-gray-100 px-3 py-1 text-sm text-gray-700 hover:bg-gray-200 pointer-coarse:min-h-11 pointer-coarse:px-4"
          }
        >
          {option.label}
        </button>
      ))}
    </fieldset>
  );
}
