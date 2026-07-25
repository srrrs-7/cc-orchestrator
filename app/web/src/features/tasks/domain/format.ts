/**
 * Pure formatting helpers for task display values.
 * No React, no DOM — safe to use from domain tests.
 */

/** Formats an ISO-8601 timestamp for human-readable display. */
export function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}
