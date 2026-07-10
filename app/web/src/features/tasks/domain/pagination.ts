/**
 * Domain layer for offset/limit pagination (SPEC-008). Dependency-free
 * like domain/task.ts: no React, no fetch, no DOM, no zod. Only plain
 * types and pure functions, reusable by any paged resource (not just
 * tasks).
 *
 * `DEFAULT_LIMIT`/`MAX_LIMIT` mirror app/api's domain value object
 * (app/api/domain/task/page.go: `task.NewPage`), which is the single
 * source of truth for the business rule (server clamps/validates
 * regardless of what the client sends). Keeping the same numbers here
 * only lets the URL search schema (src/app/router.tsx) pick sensible
 * defaults/fallbacks before a request is even made; the values actually
 * in effect always come back from the server (see `PageInfo`, echoed on
 * every `GET /tasks` response as `limit`/`offset`).
 */
export const DEFAULT_LIMIT = 20;
export const MAX_LIMIT = 100;

/**
 * The pagination metadata every paged response carries, independent of
 * what the items are. `total` is the count before paging is applied.
 */
export type PageInfo = {
  readonly total: number;
  readonly limit: number;
  readonly offset: number;
};

/** Whether a page before the current one exists. */
export function hasPreviousPage(page: PageInfo): boolean {
  return page.offset > 0;
}

/** Whether a page after the current one exists. */
export function hasNextPage(page: PageInfo): boolean {
  return page.offset + page.limit < page.total;
}

/** The offset to request for the previous page. Never negative. */
export function previousOffset(page: PageInfo): number {
  return Math.max(0, page.offset - page.limit);
}

/** The offset to request for the next page. */
export function nextOffset(page: PageInfo): number {
  return page.offset + page.limit;
}

/**
 * The 1-based, inclusive range of items shown on the current page (for
 * display, e.g. "21-40 of 57"). Returns `{from: 0, to: 0}` when the
 * page is empty, since there is no meaningful 1-based range to show.
 */
export function currentRange(page: PageInfo): { readonly from: number; readonly to: number } {
  if (page.total === 0) {
    return { from: 0, to: 0 };
  }
  return {
    from: page.offset + 1,
    to: Math.min(page.offset + page.limit, page.total),
  };
}
