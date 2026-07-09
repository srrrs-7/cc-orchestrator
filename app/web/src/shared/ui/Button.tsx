import type { ButtonHTMLAttributes, ReactNode } from "react";

type ButtonVariant = "primary" | "secondary";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  readonly variant?: ButtonVariant;
  readonly children: ReactNode;
};

const VARIANT_CLASS_NAMES: Record<ButtonVariant, string> = {
  primary: "bg-blue-600 text-white hover:bg-blue-700 disabled:bg-blue-300",
  secondary:
    "bg-gray-200 text-gray-900 hover:bg-gray-300 disabled:bg-gray-100 disabled:text-gray-400",
};

/** Generic, presentation-only button. Holds no business logic. */
export function Button({ variant = "primary", className, children, ...rest }: ButtonProps) {
  const variantClassName = VARIANT_CLASS_NAMES[variant];
  const composedClassName =
    className === undefined ? variantClassName : `${variantClassName} ${className}`;

  return (
    <button
      {...rest}
      className={`rounded px-3 py-1.5 text-sm font-medium transition-colors motion-reduce:transition-none disabled:cursor-not-allowed pointer-coarse:min-h-11 pointer-coarse:px-4 ${composedClassName}`}
    >
      {children}
    </button>
  );
}
