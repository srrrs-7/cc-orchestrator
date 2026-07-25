import { Card } from "../../../shared/ui/Card";
import { Skeleton } from "../../../shared/ui/Skeleton";

/** Placeholder shown while a single task is loading. */
export function TaskDetailSkeleton() {
  return (
    <div role="status" aria-live="polite" aria-busy="true" className="flex min-w-0 flex-col gap-4">
      <span className="sr-only">Loading task</span>
      <Skeleton className="h-4 w-24" />
      <Skeleton className="h-8 w-full max-w-md" />
      <Card className="flex flex-col gap-4 p-4">
        <div className="flex flex-wrap gap-2">
          <Skeleton className="h-5 w-14 rounded-full" />
          <Skeleton className="h-5 w-16 rounded-full" />
        </div>
        <Skeleton className="h-4 w-48" />
        <div className="flex flex-wrap gap-2">
          <Skeleton className="h-9 w-16 pointer-coarse:min-h-11" />
          <Skeleton className="h-9 w-20 pointer-coarse:min-h-11" />
        </div>
      </Card>
    </div>
  );
}
