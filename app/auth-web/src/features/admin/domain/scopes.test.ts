import { describe, expect, it } from "vitest";
import { isKnownScope, normalizeSelectedScopes } from "./scopes";

describe("normalizeSelectedScopes", () => {
  it("adds openid when missing", () => {
    expect(normalizeSelectedScopes(["profile"])).toEqual(["openid", "profile"]);
  });

  it("defaults to openid when empty", () => {
    expect(normalizeSelectedScopes([])).toEqual(["openid"]);
  });

  it("deduplicates scopes", () => {
    expect(normalizeSelectedScopes(["openid", "openid", "email"])).toEqual(["openid", "email"]);
  });
});

describe("isKnownScope", () => {
  it("accepts supported scopes", () => {
    expect(isKnownScope("offline_access")).toBe(true);
  });

  it("rejects unknown scopes", () => {
    expect(isKnownScope("admin")).toBe(false);
  });
});
