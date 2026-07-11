import { afterEach, describe, expect, it } from "vitest";
import {
  SESSION_KEYS,
  clearAllAuthStorage,
  clearReturnTo,
  clearStoredPkce,
  clearStoredSession,
  getCurrentAccessToken,
  getReturnTo,
  getStoredPkce,
  getStoredSession,
  isAuthenticated,
  isSessionValid,
  setReturnTo,
  setStoredPkce,
  setStoredSession,
} from "./session";
import type { StoredSession } from "./session";

function makeSession(overrides?: Partial<StoredSession>): StoredSession {
  return {
    accessToken: "access-token",
    idToken: "id-token",
    expiresAt: Date.now() + 3_600_000,
    sub: "user-1",
    displayName: "Test User",
    ...overrides,
  };
}

afterEach(() => {
  sessionStorage.clear();
});

describe("stored session", () => {
  it("returns null when storage is empty", () => {
    expect(getStoredSession()).toBeNull();
  });

  it("round-trips a session through sessionStorage", () => {
    const session = makeSession();
    setStoredSession(session);
    expect(getStoredSession()).toEqual(session);
  });

  it("clears the session", () => {
    setStoredSession(makeSession());
    clearStoredSession();
    expect(getStoredSession()).toBeNull();
  });

  it("returns null for malformed JSON in storage", () => {
    sessionStorage.setItem(SESSION_KEYS.session, "{invalid json}");
    expect(getStoredSession()).toBeNull();
  });

  it("returns null for a valid JSON object that fails the schema", () => {
    sessionStorage.setItem(SESSION_KEYS.session, JSON.stringify({ foo: "bar" }));
    expect(getStoredSession()).toBeNull();
  });
});

describe("stored PKCE state", () => {
  it("returns null when storage is empty", () => {
    expect(getStoredPkce()).toBeNull();
  });

  it("round-trips PKCE state through sessionStorage", () => {
    setStoredPkce({ verifier: "v", state: "s" });
    expect(getStoredPkce()).toEqual({ verifier: "v", state: "s" });
  });

  it("clears PKCE state", () => {
    setStoredPkce({ verifier: "v", state: "s" });
    clearStoredPkce();
    expect(getStoredPkce()).toBeNull();
  });
});

describe("return-to", () => {
  it("returns '/' when nothing is stored", () => {
    expect(getReturnTo()).toBe("/");
  });

  it("round-trips the return-to path", () => {
    setReturnTo("/tasks/123");
    expect(getReturnTo()).toBe("/tasks/123");
  });

  it("clears the return-to value", () => {
    setReturnTo("/tasks/123");
    clearReturnTo();
    expect(getReturnTo()).toBe("/");
  });
});

describe("isSessionValid", () => {
  it("returns true for a session that has not expired", () => {
    expect(isSessionValid(makeSession({ expiresAt: Date.now() + 1000 }))).toBe(true);
  });

  it("returns false for an expired session", () => {
    expect(isSessionValid(makeSession({ expiresAt: Date.now() - 1 }))).toBe(false);
  });
});

describe("isAuthenticated", () => {
  it("returns false when there is no session", () => {
    expect(isAuthenticated()).toBe(false);
  });

  it("returns true for a valid, non-expired session", () => {
    setStoredSession(makeSession());
    expect(isAuthenticated()).toBe(true);
  });

  it("returns false for an expired session", () => {
    setStoredSession(makeSession({ expiresAt: Date.now() - 1 }));
    expect(isAuthenticated()).toBe(false);
  });
});

describe("getCurrentAccessToken", () => {
  it("returns null when there is no session", () => {
    expect(getCurrentAccessToken()).toBeNull();
  });

  it("returns the access token for a valid session", () => {
    setStoredSession(makeSession({ accessToken: "my-token" }));
    expect(getCurrentAccessToken()).toBe("my-token");
  });

  it("returns null for an expired session", () => {
    setStoredSession(makeSession({ expiresAt: Date.now() - 1, accessToken: "my-token" }));
    expect(getCurrentAccessToken()).toBeNull();
  });
});

describe("clearAllAuthStorage", () => {
  it("removes session, PKCE, and return-to from storage", () => {
    setStoredSession(makeSession());
    setStoredPkce({ verifier: "v", state: "s" });
    setReturnTo("/tasks/1");

    clearAllAuthStorage();

    expect(getStoredSession()).toBeNull();
    expect(getStoredPkce()).toBeNull();
    expect(getReturnTo()).toBe("/");
  });
});
