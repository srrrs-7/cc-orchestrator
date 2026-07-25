import type { ReactNode } from "react";

type BadgeVariant =
  | "todo"
  | "doing"
  | "done"
  | "priority-low"
  | "priority-medium"
  | "priority-high"
  | "neutral";

type BadgeProps = {
  readonly variant?: BadgeVariant;
  readonly children: ReactNode;
  readonly className?: string;
};

const VARIANT_CLASS_NAMES: Record<BadgeVariant, string> = {
  todo: "bg-status-todo-bg text-status-todo",
  doing: "bg-status-doing-bg text-status-doing",
  done: "bg-status-done-bg text-status-done",
  "priority-low": "bg-priority-low-bg text-priority-low",
  "priority-medium": "bg-priority-medium-bg text-priority-medium",
  "priority-high": "bg-priority-high-bg text-priority-high",
  neutral: "bg-gray-100 text-gray-600",
};

/** Small label chip for status, priority, or other metadata. */
export function Badge({ variant = "neutral", className, children }: BadgeProps) {
  const variantClassName = VARIANT_CLASS_NAMES[variant];
  const composedClassName =
    className === undefined ? variantClassName : `${variantClassName} ${className}`;

  return (
    <span
      className={`inline-flex max-w-full items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${composedClassName}`}
    >
      {children}
    </span>
  );
}
