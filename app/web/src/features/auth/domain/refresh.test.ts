import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchDiscovery, refreshToken } from "../api/oidc";
import {
  REFRESH_LEAD_TIME_MS,
  ensureAuthenticatedSession,
  ensureValidAccessToken,
  isAccessTokenValid,
  refreshStoredSession,
  shouldRefreshSession,
} from "./refresh";
import { getStoredSession, setStoredSession, type StoredSession } from "./session";

vi.mock("../api/oidc", () => ({
  fetchDiscovery: vi.fn(),
  refreshToken: vi.fn(),
}));

function makeSession(overrides?: Partial<StoredSession>): StoredSession {
  return {
    accessToken: "access-token",
    idToken: makeIdToken(),
    refreshToken: "refresh-token",
    expiresAt: Date.now() + 3_600_000,
    sub: "user-1",
    displayName: "Test User",
    ...overrides,
  };
}

function makeIdToken(payload: Record<string, unknown> = {}): string {
  const header = btoa(JSON.stringify({ alg: "RS256", typ: "JWT" }))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
  const body = btoa(
    JSON.stringify({
      sub: "user-1",
      name: "Test User",
      exp: Math.floor(Date.now() / 1000) + 3600,
      ...payload,
    }),
  )
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
  return `${header}.${body}.fake-signature`;
}

beforeEach(() => {
  vi.mocked(fetchDiscovery).mockResolvedValue({
    issuer: "http://localhost/auth",
    authorization_endpoint: "http://localhost/auth/authorize",
    token_endpoint: "http://localhost/auth/token",
  });
  vi.mocked(refreshToken).mockResolvedValue({
    access_token: "new-access-token",
    id_token: makeIdToken({ name: "Refreshed User" }),
    refresh_token: "new-refresh-token",
    expires_in: 3600,
    token_type: "Bearer",
  });
});

afterEach(() => {
  sessionStorage.clear();
  vi.clearAllMocks();
});

describe("shouldRefreshSession", () => {
  it("returns false when no refresh token is stored", () => {
    const session = makeSession({ refreshToken: undefined });
    expect(shouldRefreshSession(session)).toBe(false);
  });

  it("returns true when expiry is within the lead time", () => {
    const now = Date.now();
    const session = makeSession({ expiresAt: now + REFRESH_LEAD_TIME_MS - 1 });
    expect(shouldRefreshSession(session, now)).toBe(true);
  });

  it("returns false when expiry is beyond the lead time", () => {
    const now = Date.now();
    const session = makeSession({ expiresAt: now + REFRESH_LEAD_TIME_MS + 60_000 });
    expect(shouldRefreshSession(session, now)).toBe(false);
  });
});

describe("refreshStoredSession", () => {
  it("updates sessionStorage with rotated tokens", async () => {
    setStoredSession(makeSession({ expiresAt: Date.now() - 1 }));

    const refreshed = await refreshStoredSession();

    expect(refreshed?.accessToken).toBe("new-access-token");
    expect(getStoredSession()?.refreshToken).toBe("new-refresh-token");
    expect(refreshToken).toHaveBeenCalledOnce();
  });

  it("returns null when refresh token is absent", async () => {
    setStoredSession(makeSession({ refreshToken: undefined, expiresAt: Date.now() - 1 }));

    const refreshed = await refreshStoredSession();

    expect(refreshed).toBeNull();
    expect(refreshToken).not.toHaveBeenCalled();
  });
});

describe("ensureValidAccessToken", () => {
  it("returns the current token without calling refresh when still valid", async () => {
    setStoredSession(makeSession());

    const token = await ensureValidAccessToken();

    expect(token).toBe("access-token");
    expect(refreshToken).not.toHaveBeenCalled();
  });

  it("refreshes and returns a new token when expired", async () => {
    setStoredSession(makeSession({ expiresAt: Date.now() - 1 }));

    const token = await ensureValidAccessToken();

    expect(token).toBe("new-access-token");
    expect(refreshToken).toHaveBeenCalledOnce();
  });
});

describe("ensureAuthenticatedSession", () => {
  it("returns true for a valid non-expired session", async () => {
    setStoredSession(makeSession());
    await expect(ensureAuthenticatedSession()).resolves.toBe(true);
  });

  it("returns false when expired and refresh fails", async () => {
    setStoredSession(makeSession({ expiresAt: Date.now() - 1 }));
    vi.mocked(refreshToken).mockRejectedValueOnce(new Error("invalid_grant"));

    await expect(ensureAuthenticatedSession()).resolves.toBe(false);
  });

  it("returns true after refreshing an expired session", async () => {
    setStoredSession(makeSession({ expiresAt: Date.now() - 1 }));

    await expect(ensureAuthenticatedSession()).resolves.toBe(true);
    const stored = getStoredSession();
    expect(stored).not.toBeNull();
    if (stored) {
      expect(isAccessTokenValid(stored)).toBe(true);
    }
  });
});
