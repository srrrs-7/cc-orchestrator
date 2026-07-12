import { describe, expect, it } from "vitest";
import { formatRedirectUris, parseRedirectUris } from "./redirectUris";

describe("redirectUris", () => {
  it("parses newline-separated URIs", () => {
    expect(parseRedirectUris("http://a.test/cb\nhttp://b.test/cb\n")).toEqual([
      "http://a.test/cb",
      "http://b.test/cb",
    ]);
  });

  it("deduplicates URIs", () => {
    expect(parseRedirectUris("http://a.test/cb, http://a.test/cb")).toEqual(["http://a.test/cb"]);
  });

  it("formats URIs for textarea display", () => {
    expect(formatRedirectUris(["http://a.test/cb", "http://b.test/cb"])).toBe(
      "http://a.test/cb\nhttp://b.test/cb",
    );
  });
});
