import { describe, expect, it } from "vitest";
import { deriveCodeChallenge, generateCodeVerifier } from "./pkce";

describe("generateCodeVerifier", () => {
  it("returns a base64url string of 43 characters (32 bytes)", async () => {
    const verifier = await generateCodeVerifier();
    // 32 bytes → 43 base64url chars (ceil(32 * 4/3) with no padding)
    expect(verifier).toHaveLength(43);
  });

  it("only contains base64url-safe characters (no +, /, or =)", async () => {
    const verifier = await generateCodeVerifier();
    expect(verifier).toMatch(/^[A-Za-z0-9\-_]+$/);
  });

  it("returns a different value on each call (randomness)", async () => {
    const a = await generateCodeVerifier();
    const b = await generateCodeVerifier();
    expect(a).not.toBe(b);
  });
});

describe("deriveCodeChallenge", () => {
  it("returns a non-empty base64url string", async () => {
    const verifier = await generateCodeVerifier();
    const challenge = await deriveCodeChallenge(verifier);
    expect(challenge.length).toBeGreaterThan(0);
    expect(challenge).toMatch(/^[A-Za-z0-9\-_]+$/);
  });

  it("is deterministic for the same verifier", async () => {
    const verifier = "fixed-test-verifier-string-for-determinism";
    const a = await deriveCodeChallenge(verifier);
    const b = await deriveCodeChallenge(verifier);
    expect(a).toBe(b);
  });

  it("produces different challenges for different verifiers", async () => {
    const a = await deriveCodeChallenge("verifier-one");
    const b = await deriveCodeChallenge("verifier-two");
    expect(a).not.toBe(b);
  });
});
