import { useNavigate } from "@tanstack/react-router";
import { Button } from "../../../shared/ui/Button";
import type { PageInfo } from "../domain/pagination";
import {
  currentRange,
  hasNextPage,
  hasPreviousPage,
  nextOffset,
  previousOffset,
} from "../domain/pagination";

type TaskPagerProps = {
  readonly page: PageInfo;
};

/**
 * Minimal next/previous pager for the task list (SPEC-008 R6). Scope is
 * deliberately limited to next/previous -- no page-number jump, no
 * infinite scroll (see SPEC-008 §3 "スコープ外").
 *
 * Activation state and the displayed range are derived from the
 * server-echoed `page` (domain/pagination.ts), not from the URL search
 * params directly, so the buttons reflect what the server actually
 * applied (e.g. a clamped `limit`) rather than what was requested.
 */
export function TaskPager({ page }: TaskPagerProps) {
  const navigate = useNavigate({ from: "/" });
  const { from, to } = currentRange(page);

  if (page.total === 0) {
    return null;
  }

  return (
    <nav
      aria-label="Task list pages"
      className="flex flex-wrap items-center justify-between gap-2 text-sm text-gray-600"
    >
      <p>
        {from}-{to} of {page.total}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="secondary"
          onClick={() =>
            navigate({ search: (prev) => ({ ...prev, offset: previousOffset(page) }) })
          }
          disabled={!hasPreviousPage(page)}
        >
          Previous
        </Button>
        <Button
          type="button"
          variant="secondary"
          onClick={() => navigate({ search: (prev) => ({ ...prev, offset: nextOffset(page) }) })}
          disabled={!hasNextPage(page)}
        >
          Next
        </Button>
      </div>
    </nav>
  );
}
