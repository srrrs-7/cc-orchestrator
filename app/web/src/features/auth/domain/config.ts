/**
 * OIDC client configuration resolved from Vite environment variables with
 * sensible defaults for the local dev setup.
 *
 * Env vars (set in .env / .env.local — never commit secrets):
 *   VITE_AUTH_CLIENT_ID    default: demo-client
 *   VITE_AUTH_ISSUER       default: <origin>/auth
 *   VITE_AUTH_REDIRECT_URI default: <origin>/callback
 *   VITE_AUTH_SCOPES       default: openid profile email  (space-separated)
 */

export type AuthConfig = {
  readonly issuer: string;
  readonly clientId: string;
  readonly redirectUri: string;
  readonly scopes: readonly string[];
};

export function resolveAuthConfig(): AuthConfig {
  const origin = window.location.origin;

  const rawIssuer = import.meta.env.VITE_AUTH_ISSUER;
  const issuer =
    typeof rawIssuer === "string" && rawIssuer.length > 0 ? rawIssuer : `${origin}/auth`;

  const rawClientId = import.meta.env.VITE_AUTH_CLIENT_ID;
  const clientId =
    typeof rawClientId === "string" && rawClientId.length > 0 ? rawClientId : "demo-client";

  const rawRedirectUri = import.meta.env.VITE_AUTH_REDIRECT_URI;
  const redirectUri =
    typeof rawRedirectUri === "string" && rawRedirectUri.length > 0
      ? rawRedirectUri
      : `${origin}/callback`;

  const rawScopes = import.meta.env.VITE_AUTH_SCOPES;
  const scopes =
    typeof rawScopes === "string" && rawScopes.length > 0
      ? rawScopes.split(" ").filter((s) => s.length > 0)
      : ["openid", "profile", "email"];

  return { issuer, clientId, redirectUri, scopes };
}
