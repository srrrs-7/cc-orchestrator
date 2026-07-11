/**
 * PKCE (Proof Key for Code Exchange) helpers for Authorization Code + S256
 * flow (RFC 7636). Uses the Web Crypto API — available in all modern browsers
 * and in the jsdom test environment via Node's built-in `crypto`.
 */

function base64urlEncode(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
}

/**
 * Generate a cryptographically random `code_verifier` (RFC 7636 §4.1).
 * 32 random bytes → 43-character base64url string (no padding).
 */
export async function generateCodeVerifier(): Promise<string> {
  const buffer = new Uint8Array(32);
  crypto.getRandomValues(buffer);
  return base64urlEncode(buffer);
}

/**
 * Derive the S256 `code_challenge` from a `code_verifier` (RFC 7636 §4.2).
 * SHA-256 digest of the ASCII verifier → base64url-encoded (no padding).
 */
export async function deriveCodeChallenge(verifier: string): Promise<string> {
  const encoded = new TextEncoder().encode(verifier);
  const digest = await crypto.subtle.digest("SHA-256", encoded);
  return base64urlEncode(new Uint8Array(digest));
}
