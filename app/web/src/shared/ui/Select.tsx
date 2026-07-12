import type { SelectHTMLAttributes } from "react";

type SelectProps = SelectHTMLAttributes<HTMLSelectElement>;

const SELECT_CLASS_NAME =
  "w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus-visible:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 pointer-coarse:min-h-11";

/** Styled select with accessible focus ring. */
export function Select({ className, children, ...rest }: SelectProps) {
  const composedClassName =
    className === undefined ? SELECT_CLASS_NAME : `${SELECT_CLASS_NAME} ${className}`;

  return (
    <select {...rest} className={composedClassName}>
      {children}
    </select>
  );
}
