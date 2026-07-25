import { z } from "zod";

/**
 * sessionStorage keys used by the auth feature. Grouped here so they can
 * be imported by tests / other modules without duplicating the strings.
 */
export const SESSION_KEYS = {
  session: "auth.session",
  pkce: "auth.pkce",
  returnTo: "auth.return_to",
} as const;

const storedSessionSchema = z.object({
  accessToken: z.string(),
  idToken: z.string(),
  refreshToken: z.string().optional(),
  /** Unix timestamp in milliseconds when the access token expires. */
  expiresAt: z.number(),
  sub: z.string(),
  displayName: z.string(),
});

export type StoredSession = z.infer<typeof storedSessionSchema>;

const storedPkceSchema = z.object({
  verifier: z.string(),
  state: z.string(),
});

export type StoredPkce = z.infer<typeof storedPkceSchema>;

// ---------------------------------------------------------------------------
// Session (tokens + user info)
// ---------------------------------------------------------------------------

export function getStoredSession(): StoredSession | null {
  try {
    const raw = sessionStorage.getItem(SESSION_KEYS.session);
    if (!raw) return null;
    return storedSessionSchema.parse(JSON.parse(raw));
  } catch {
    return null;
  }
}

export function setStoredSession(session: StoredSession): void {
  sessionStorage.setItem(SESSION_KEYS.session, JSON.stringify(session));
}

export function clearStoredSession(): void {
  sessionStorage.removeItem(SESSION_KEYS.session);
}

// ---------------------------------------------------------------------------
// PKCE state (verifier + nonce state stored before the OIDC redirect)
// ---------------------------------------------------------------------------

export function getStoredPkce(): StoredPkce | null {
  try {
    const raw = sessionStorage.getItem(SESSION_KEYS.pkce);
    if (!raw) return null;
    return storedPkceSchema.parse(JSON.parse(raw));
  } catch {
    return null;
  }
}

export function setStoredPkce(pkce: StoredPkce): void {
  sessionStorage.setItem(SESSION_KEYS.pkce, JSON.stringify(pkce));
}

export function clearStoredPkce(): void {
  sessionStorage.removeItem(SESSION_KEYS.pkce);
}

// ---------------------------------------------------------------------------
// Return-to path (stored before redirecting to /login, restored after callback)
// ---------------------------------------------------------------------------

export function setReturnTo(path: string): void {
  sessionStorage.setItem(SESSION_KEYS.returnTo, path);
}

export function getReturnTo(): string {
  return sessionStorage.getItem(SESSION_KEYS.returnTo) ?? "/";
}

export function clearReturnTo(): void {
  sessionStorage.removeItem(SESSION_KEYS.returnTo);
}

// ---------------------------------------------------------------------------
// Helpers used from non-React code (router beforeLoad, API client)
// ---------------------------------------------------------------------------

export function isSessionValid(session: StoredSession): boolean {
  return Date.now() < session.expiresAt;
}

/** Used by the router beforeLoad guard — synchronous, no React context required. */
export function isAuthenticated(): boolean {
  const session = getStoredSession();
  return session !== null && isSessionValid(session);
}

/** Used by the API client interceptor — synchronous, no React context required. */
export function getCurrentAccessToken(): string | null {
  const session = getStoredSession();
  if (session === null || !isSessionValid(session)) return null;
  return session.accessToken;
}

/** Clear everything auth-related from sessionStorage. */
export function clearAllAuthStorage(): void {
  sessionStorage.removeItem(SESSION_KEYS.session);
  sessionStorage.removeItem(SESSION_KEYS.pkce);
  sessionStorage.removeItem(SESSION_KEYS.returnTo);
}
