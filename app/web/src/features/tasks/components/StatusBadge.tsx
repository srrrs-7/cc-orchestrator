import { Badge } from "../../../shared/ui/Badge";
import type { TaskStatus } from "../domain/task";

const STATUS_LABELS: Record<TaskStatus, string> = {
  todo: "Todo",
  doing: "Doing",
  done: "Done",
};

type StatusBadgeProps = {
  readonly status: TaskStatus;
};

/** Color-coded chip for a task's workflow status. */
export function StatusBadge({ status }: StatusBadgeProps) {
  return <Badge variant={status}>{STATUS_LABELS[status]}</Badge>;
}
