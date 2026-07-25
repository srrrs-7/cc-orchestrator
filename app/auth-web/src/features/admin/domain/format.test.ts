import { describe, expect, it } from "vitest";
import { formatScopeList, formatUriList } from "./format";

describe("format", () => {
  it("formats empty scope lists", () => {
    expect(formatScopeList([])).toBe("—");
  });

  it("joins scopes for display", () => {
    expect(formatScopeList(["openid", "profile"])).toBe("openid, profile");
  });

  it("formats redirect URIs on separate lines", () => {
    expect(formatUriList(["http://a.test/cb", "http://b.test/cb"])).toBe(
      "http://a.test/cb\nhttp://b.test/cb",
    );
  });
});
