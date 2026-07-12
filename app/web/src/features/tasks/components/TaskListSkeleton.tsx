import { Card } from "../../../shared/ui/Card";
import { Skeleton } from "../../../shared/ui/Skeleton";

/** Placeholder cards shown while the task list is loading. */
export function TaskListSkeleton() {
  return (
    <div role="status" aria-live="polite" aria-busy="true" className="flex flex-col gap-3">
      <span className="sr-only">Loading tasks</span>
      {[0, 1, 2].map((key) => (
        <Card
          key={key}
          className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"
        >
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <Skeleton className="h-5 w-3/4 max-w-xs" />
            <div className="flex flex-wrap gap-2">
              <Skeleton className="h-5 w-14 rounded-full" />
              <Skeleton className="h-5 w-16 rounded-full" />
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Skeleton className="h-9 w-16 pointer-coarse:min-h-11" />
            <Skeleton className="h-9 w-20 pointer-coarse:min-h-11" />
          </div>
        </Card>
      ))}
    </div>
  );
}
