import type { ReactNode } from "react";

type BadgeProps = {
  readonly children: ReactNode;
  readonly className?: string;
};

export function Badge({ className, children }: BadgeProps) {
  const base =
    "inline-flex max-w-full items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-700";
  return (
    <span className={className === undefined ? base : `${base} ${className}`}>{children}</span>
  );
}
