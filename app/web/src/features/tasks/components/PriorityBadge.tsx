import { Badge } from "../../../shared/ui/Badge";
import type { TaskPriority } from "../domain/task";

const PRIORITY_LABELS: Record<TaskPriority, string> = {
  low: "Low",
  medium: "Medium",
  high: "High",
};

const PRIORITY_VARIANTS = {
  low: "priority-low",
  medium: "priority-medium",
  high: "priority-high",
} as const;

type PriorityBadgeProps = {
  readonly priority: TaskPriority;
};

/** Color-coded chip for a task's priority level. */
export function PriorityBadge({ priority }: PriorityBadgeProps) {
  return <Badge variant={PRIORITY_VARIANTS[priority]}>{PRIORITY_LABELS[priority]}</Badge>;
}
