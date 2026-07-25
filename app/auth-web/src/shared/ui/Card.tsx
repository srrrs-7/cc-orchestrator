import type { HTMLAttributes, ReactNode } from "react";

type CardProps = HTMLAttributes<HTMLDivElement> & {
  readonly children: ReactNode;
};

export function Card({ children, className, ...rest }: CardProps) {
  const baseClassName = "rounded-lg border border-border-subtle bg-surface shadow-sm";
  const composedClassName =
    className === undefined ? baseClassName : `${baseClassName} ${className}`;

  return (
    <div {...rest} className={composedClassName}>
      {children}
    </div>
  );
}

type CardHeaderProps = {
  readonly title: string;
  readonly description?: string;
  readonly className?: string;
};

export function CardHeader({ title, description, className }: CardHeaderProps) {
  const composedClassName = className === undefined ? "mb-4" : `mb-4 ${className}`;

  return (
    <div className={composedClassName}>
      <h2 className="text-base font-semibold text-gray-900">{title}</h2>
      {description !== undefined ? (
        <p className="mt-1 text-sm text-gray-500">{description}</p>
      ) : null}
    </div>
  );
}
