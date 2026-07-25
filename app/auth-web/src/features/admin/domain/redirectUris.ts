/** Parses a newline- or comma-separated list of redirect URIs. */
export function parseRedirectUris(raw: string): string[] {
  return [
    ...new Set(
      raw
        .split(/[\n,]/)
        .map((line) => line.trim())
        .filter((line) => line.length > 0),
    ),
  ];
}

/** Formats redirect URIs for display in a textarea. */
export function formatRedirectUris(uris: readonly string[]): string {
  return uris.join("\n");
}
