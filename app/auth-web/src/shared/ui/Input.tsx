import type { InputHTMLAttributes } from "react";

type InputProps = InputHTMLAttributes<HTMLInputElement>;

const INPUT_CLASS_NAME =
  "w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus-visible:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 pointer-coarse:min-h-11";

export function Input({ className, ...rest }: InputProps) {
  const composedClassName =
    className === undefined ? INPUT_CLASS_NAME : `${INPUT_CLASS_NAME} ${className}`;

  return <input {...rest} className={composedClassName} />;
}
