import type { TextareaHTMLAttributes } from "react";

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

const TEXTAREA_CLASS_NAME =
  "w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus-visible:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30";

export function Textarea({ className, ...rest }: TextareaProps) {
  const composedClassName =
    className === undefined ? TEXTAREA_CLASS_NAME : `${TEXTAREA_CLASS_NAME} ${className}`;

  return <textarea {...rest} className={composedClassName} />;
}
