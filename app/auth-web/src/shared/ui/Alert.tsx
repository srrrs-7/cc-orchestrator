import type { ReactNode } from "react";

type AlertVariant = "error" | "info" | "success";

type AlertProps = {
  readonly variant?: AlertVariant;
  readonly children: ReactNode;
  readonly className?: string;
};

const VARIANT_CLASS_NAMES: Record<AlertVariant, string> = {
  error: "border-red-200 bg-red-50 text-red-800",
  info: "border-blue-200 bg-blue-50 text-blue-800",
  success: "border-green-200 bg-green-50 text-green-800",
};

export function Alert({ variant = "error", className, children }: AlertProps) {
  const variantClassName = VARIANT_CLASS_NAMES[variant];
  const composedClassName =
    className === undefined
      ? `rounded-lg border p-4 text-sm ${variantClassName}`
      : `rounded-lg border p-4 text-sm ${variantClassName} ${className}`;

  return (
    <div role="alert" className={composedClassName}>
      {children}
    </div>
  );
}
