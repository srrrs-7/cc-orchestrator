import { describe, expect, it } from "vitest";
import { resolveAuthConfig } from "./config";

/**
 * `resolveAuthConfig` reads from `import.meta.env` and `window.location.origin`.
 * In the jsdom test environment, `window.location.origin` is "http://localhost"
 * and no VITE_AUTH_* vars are set, so all values fall back to the defaults.
 */
describe("resolveAuthConfig (defaults)", () => {
  it("uses origin/auth as the default issuer", () => {
    const { issuer } = resolveAuthConfig();
    expect(issuer).toBe(`${window.location.origin}/auth`);
  });

  it("defaults the client ID to demo-client", () => {
    const { clientId } = resolveAuthConfig();
    expect(clientId).toBe("demo-client");
  });

  it("defaults the redirect URI to origin/callback", () => {
    const { redirectUri } = resolveAuthConfig();
    expect(redirectUri).toBe(`${window.location.origin}/callback`);
  });

  it("defaults the scopes to openid, profile, email, offline_access", () => {
    const { scopes } = resolveAuthConfig();
    expect(scopes).toEqual(["openid", "profile", "email", "offline_access"]);
  });
});
