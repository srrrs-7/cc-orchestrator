/**
 * JWT payload parsing helpers for display purposes.
 *
 * NOTE: No signature verification is performed here. The browser only uses
 * the ID token payload to extract display information (name, sub). The
 * authoritative identity check happens server-side when the access token is
 * sent on API requests. The id_token is validated by the auth server before
 * issuance; we trust that the TLS channel protected it in transit.
 */

/** Decode the base64url-encoded payload segment of a JWT. */
export function parseJwtPayload(jwt: string): Record<string, unknown> {
  const parts = jwt.split(".");
  const payload = parts[1];
  if (!payload) {
    throw new Error("Invalid JWT: missing payload segment");
  }
  // base64url → standard base64 → binary string → JSON
  const base64 = payload.replace(/-/g, "+").replace(/_/g, "/");
  const json = atob(base64);
  return JSON.parse(json) as Record<string, unknown>;
}

/** Extract a human-readable display name from the ID token payload. */
export function extractDisplayName(payload: Record<string, unknown>): string {
  const name = payload.name;
  if (typeof name === "string" && name.length > 0) return name;
  const email = payload.email;
  if (typeof email === "string" && email.length > 0) return email;
  const sub = payload.sub;
  return typeof sub === "string" && sub.length > 0 ? sub : "User";
}

/** Extract the `sub` (subject) claim — throws if absent or non-string. */
export function extractSub(payload: Record<string, unknown>): string {
  const sub = payload.sub;
  if (typeof sub !== "string" || sub.length === 0) {
    throw new Error("Invalid ID token: missing or empty sub claim");
  }
  return sub;
}

/**
 * Extract the token expiry as a Unix timestamp in **milliseconds**.
 * The JWT `exp` claim is in seconds (RFC 7519 §4.1.4).
 */
export function extractExpiry(payload: Record<string, unknown>): number {
  const exp = payload.exp;
  if (typeof exp !== "number") {
    throw new Error("Invalid ID token: missing or non-numeric exp claim");
  }
  return exp * 1000;
}
