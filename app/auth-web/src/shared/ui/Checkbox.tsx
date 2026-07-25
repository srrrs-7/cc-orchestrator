import type { InputHTMLAttributes } from "react";

type CheckboxProps = Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & {
  readonly label: string;
  readonly description?: string;
};

export function Checkbox({ label, description, className, id, ...rest }: CheckboxProps) {
  const inputId = id ?? label.toLowerCase().replace(/\s+/g, "-");

  return (
    <label
      htmlFor={inputId}
      className={`flex min-w-0 cursor-pointer items-start gap-3 rounded-md border border-border-subtle bg-surface-muted/60 p-3 transition-colors hover:bg-surface-muted pointer-coarse:min-h-11 ${className ?? ""}`}
    >
      <input
        {...rest}
        id={inputId}
        type="checkbox"
        className="mt-0.5 size-4 shrink-0 rounded border-gray-300 text-accent focus-visible:ring-2 focus-visible:ring-accent/30"
      />
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium text-gray-900">{label}</span>
        {description !== undefined ? (
          <span className="mt-0.5 block break-words text-xs text-gray-500">{description}</span>
        ) : null}
      </span>
    </label>
  );
}
