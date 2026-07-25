import { fetchDiscovery, refreshToken } from "../api/oidc";
import { resolveAuthConfig } from "./config";
import { getStoredSession, setStoredSession, type StoredSession } from "./session";
import { extractDisplayName, extractSub, parseJwtPayload } from "./token";

/** Refresh when the access token expires within this window (ms). */
export const REFRESH_LEAD_TIME_MS = 60_000;

let refreshInFlight: Promise<StoredSession | null> | null = null;

export function isAccessTokenValid(session: StoredSession, now = Date.now()): boolean {
  return now < session.expiresAt;
}

export function shouldRefreshSession(session: StoredSession, now = Date.now()): boolean {
  if (!session.refreshToken) return false;
  return now >= session.expiresAt - REFRESH_LEAD_TIME_MS;
}

async function performRefresh(session: StoredSession): Promise<StoredSession | null> {
  if (!session.refreshToken) return null;

  const config = resolveAuthConfig();
  try {
    const discovery = await fetchDiscovery(config.issuer);
    const tokens = await refreshToken({
      tokenEndpoint: discovery.token_endpoint,
      refreshToken: session.refreshToken,
      clientId: config.clientId,
    });

    const payload = parseJwtPayload(tokens.id_token);
    const updated: StoredSession = {
      accessToken: tokens.access_token,
      idToken: tokens.id_token,
      refreshToken: tokens.refresh_token ?? session.refreshToken,
      expiresAt: Date.now() + tokens.expires_in * 1000,
      sub: extractSub(payload),
      displayName: extractDisplayName(payload),
    };
    setStoredSession(updated);
    return updated;
  } catch {
    return null;
  }
}

/** Refresh the stored session once; concurrent callers share the same promise. */
export async function refreshStoredSession(): Promise<StoredSession | null> {
  if (refreshInFlight) {
    return refreshInFlight;
  }

  const session = getStoredSession();
  if (!session?.refreshToken) return null;

  refreshInFlight = performRefresh(session);
  try {
    return await refreshInFlight;
  } finally {
    refreshInFlight = null;
  }
}

/**
 * Returns a valid access token, refreshing when expired or near expiry.
 * Returns null when unauthenticated or refresh fails.
 */
export async function ensureValidAccessToken(): Promise<string | null> {
  const session = getStoredSession();
  if (!session) return null;

  if (isAccessTokenValid(session) && !shouldRefreshSession(session)) {
    return session.accessToken;
  }

  if (!session.refreshToken) return null;

  const refreshed = await refreshStoredSession();
  return refreshed?.accessToken ?? null;
}

/**
 * For route guards: returns true when a valid session exists, attempting
 * refresh when the access token is expired but a refresh token is present.
 */
export async function ensureAuthenticatedSession(): Promise<boolean> {
  const session = getStoredSession();
  if (!session) return false;

  if (isAccessTokenValid(session) && !shouldRefreshSession(session)) {
    return true;
  }

  if (!session.refreshToken) return false;

  const refreshed = await refreshStoredSession();
  return refreshed !== null;
}
