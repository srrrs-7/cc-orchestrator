import { z } from "zod";

/**
 * OIDC wire-level functions: discovery, authorize URL builder, token exchange,
 * token refresh. All external data is validated with zod before typing.
 */

// ---------------------------------------------------------------------------
// Schemas
// ---------------------------------------------------------------------------

const discoverySchema = z.object({
  issuer: z.string(),
  authorization_endpoint: z.string(),
  token_endpoint: z.string(),
  userinfo_endpoint: z.string().optional(),
  end_session_endpoint: z.string().optional(),
  jwks_uri: z.string().optional(),
});

export type OidcDiscovery = z.infer<typeof discoverySchema>;

const tokenResponseSchema = z.object({
  access_token: z.string(),
  id_token: z.string(),
  refresh_token: z.string().optional(),
  expires_in: z.number(),
  token_type: z.string(),
});

export type OidcTokenResponse = z.infer<typeof tokenResponseSchema>;

const userinfoSchema = z.object({
  sub: z.string(),
  name: z.string().optional(),
  email: z.string().optional(),
});

export type OidcUserinfo = z.infer<typeof userinfoSchema>;

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

/** Fetch and validate the OIDC discovery document. */
export async function fetchDiscovery(issuer: string): Promise<OidcDiscovery> {
  const url = `${issuer}/.well-known/openid-configuration`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`OIDC discovery failed: HTTP ${response.status}`);
  }
  const json: unknown = await response.json();
  return discoverySchema.parse(json);
}

/** Build the authorization endpoint URL with PKCE parameters. */
export function buildAuthorizeUrl(params: {
  readonly authorizationEndpoint: string;
  readonly clientId: string;
  readonly redirectUri: string;
  readonly scopes: readonly string[];
  readonly codeChallenge: string;
  readonly state: string;
}): string {
  const url = new URL(params.authorizationEndpoint);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("client_id", params.clientId);
  url.searchParams.set("redirect_uri", params.redirectUri);
  url.searchParams.set("scope", params.scopes.join(" "));
  url.searchParams.set("code_challenge", params.codeChallenge);
  url.searchParams.set("code_challenge_method", "S256");
  url.searchParams.set("state", params.state);
  return url.toString();
}

/** Build the OIDC RP-initiated logout URL. */
export function buildEndSessionUrl(params: {
  readonly endSessionEndpoint: string;
  readonly clientId: string;
  readonly idTokenHint?: string;
  readonly postLogoutRedirectUri?: string;
  readonly state?: string;
}): string {
  const url = new URL(params.endSessionEndpoint);
  url.searchParams.set("client_id", params.clientId);
  if (params.idTokenHint) {
    url.searchParams.set("id_token_hint", params.idTokenHint);
  }
  if (params.postLogoutRedirectUri) {
    url.searchParams.set("post_logout_redirect_uri", params.postLogoutRedirectUri);
  }
  if (params.state) {
    url.searchParams.set("state", params.state);
  }
  return url.toString();
}

/** Exchange an authorization code for tokens. */
export async function exchangeCode(params: {
  readonly tokenEndpoint: string;
  readonly code: string;
  readonly redirectUri: string;
  readonly clientId: string;
  readonly codeVerifier: string;
}): Promise<OidcTokenResponse> {
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code: params.code,
    redirect_uri: params.redirectUri,
    client_id: params.clientId,
    code_verifier: params.codeVerifier,
  });

  const response = await fetch(params.tokenEndpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(`Token exchange failed: HTTP ${response.status} — ${text}`);
  }

  const json: unknown = await response.json();
  return tokenResponseSchema.parse(json);
}

/** Refresh an access token using a refresh token. */
export async function refreshToken(params: {
  readonly tokenEndpoint: string;
  readonly refreshToken: string;
  readonly clientId: string;
}): Promise<OidcTokenResponse> {
  const body = new URLSearchParams({
    grant_type: "refresh_token",
    refresh_token: params.refreshToken,
    client_id: params.clientId,
  });

  const response = await fetch(params.tokenEndpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });

  if (!response.ok) {
    throw new Error(`Token refresh failed: HTTP ${response.status}`);
  }

  const json: unknown = await response.json();
  return tokenResponseSchema.parse(json);
}

/** Fetch userinfo from the OIDC userinfo endpoint. */
export async function fetchUserinfo(params: {
  readonly userinfoEndpoint: string;
  readonly accessToken: string;
}): Promise<OidcUserinfo> {
  const response = await fetch(params.userinfoEndpoint, {
    headers: { Authorization: `Bearer ${params.accessToken}` },
  });

  if (!response.ok) {
    throw new Error(`Userinfo fetch failed: HTTP ${response.status}`);
  }

  const json: unknown = await response.json();
  return userinfoSchema.parse(json);
}
