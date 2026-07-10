import { describe, expect, it } from "vitest";
import type { PageInfo } from "./pagination";
import {
  currentRange,
  hasNextPage,
  hasPreviousPage,
  nextOffset,
  previousOffset,
} from "./pagination";

// SPEC-008 R1/R2/R3/R6: these pure functions drive TaskPager's
// activation state and displayed range. Every case below is derived
// from a server-echoed PageInfo (total/limit/offset), never from the
// requested values, matching how the server (app/api's task.Page) is
// the single source of truth for what was actually applied.

describe("hasPreviousPage", () => {
  it("is false at the first page (offset=0) (boundary)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    expect(hasPreviousPage(page)).toBe(false);
  });

  it("is true once offset is greater than zero (normal)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 20 };
    expect(hasPreviousPage(page)).toBe(true);
  });
});

describe("hasNextPage", () => {
  it("is true when there are more items beyond the current page (normal)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    expect(hasNextPage(page)).toBe(true);
  });

  it("is false once offset+limit exactly reaches total (boundary: exact multiple)", () => {
    const page: PageInfo = { total: 40, limit: 20, offset: 20 };
    expect(hasNextPage(page)).toBe(false);
  });

  it("is false once offset+limit exceeds total (boundary: remainder page)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 40 };
    expect(hasNextPage(page)).toBe(false);
  });

  it("is false when total is zero (boundary: empty result set)", () => {
    const page: PageInfo = { total: 0, limit: 20, offset: 0 };
    expect(hasNextPage(page)).toBe(false);
  });

  it("is false when total is smaller than a single page (boundary: total<limit)", () => {
    const page: PageInfo = { total: 3, limit: 20, offset: 0 };
    expect(hasNextPage(page)).toBe(false);
  });
});

describe("previousOffset", () => {
  it("steps back by exactly one page (normal)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 40 };
    expect(previousOffset(page)).toBe(20);
  });

  it("never goes below zero (boundary: less than one page from the start)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 10 };
    expect(previousOffset(page)).toBe(0);
  });

  it("is zero when already at the first page (boundary)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    expect(previousOffset(page)).toBe(0);
  });
});

describe("nextOffset", () => {
  it("steps forward by exactly one page (normal)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    expect(nextOffset(page)).toBe(20);
  });

  it("can land past total when the current page is the last one (boundary)", () => {
    // nextOffset is a pure offset+limit calculation; callers gate its
    // use behind hasNextPage (see TaskPager), so it is not expected to
    // clamp to total itself.
    const page: PageInfo = { total: 45, limit: 20, offset: 40 };
    expect(nextOffset(page)).toBe(60);
  });
});

describe("currentRange", () => {
  it("returns the 1-based inclusive range for a full page (normal)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 0 };
    expect(currentRange(page)).toEqual({ from: 1, to: 20 });
  });

  it("returns the 1-based range for a middle page (normal)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 20 };
    expect(currentRange(page)).toEqual({ from: 21, to: 40 });
  });

  it("caps `to` at total for a partial final page (boundary: remainder page)", () => {
    const page: PageInfo = { total: 57, limit: 20, offset: 40 };
    expect(currentRange(page)).toEqual({ from: 41, to: 57 });
  });

  it("returns {from:0, to:0} when total is zero (boundary: empty result set)", () => {
    const page: PageInfo = { total: 0, limit: 20, offset: 0 };
    expect(currentRange(page)).toEqual({ from: 0, to: 0 });
  });

  it("covers the whole set in one page when total is exactly one page (boundary: exact multiple)", () => {
    const page: PageInfo = { total: 20, limit: 20, offset: 0 };
    expect(currentRange(page)).toEqual({ from: 1, to: 20 });
  });

  it("returns a single-item range when total is smaller than limit (boundary: total<limit)", () => {
    const page: PageInfo = { total: 3, limit: 20, offset: 0 };
    expect(currentRange(page)).toEqual({ from: 1, to: 3 });
  });
});
