type SkeletonProps = {
  readonly className?: string;
};

const BASE_CLASS_NAME = "animate-pulse rounded-md bg-gray-200 motion-reduce:animate-none";

/** Placeholder block for loading states. */
export function Skeleton({ className }: SkeletonProps) {
  const composedClassName =
    className === undefined ? BASE_CLASS_NAME : `${BASE_CLASS_NAME} ${className}`;

  return <div aria-hidden="true" className={composedClassName} />;
}
