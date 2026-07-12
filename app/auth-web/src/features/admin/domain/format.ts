/** Joins scope labels for compact table display. */
export function formatScopeList(scopes: readonly string[]): string {
  if (scopes.length === 0) {
    return "—";
  }
  return scopes.join(", ");
}

/** Joins URIs with line breaks for readable list cells. */
export function formatUriList(uris: readonly string[]): string {
  if (uris.length === 0) {
    return "—";
  }
  return uris.join("\n");
}
