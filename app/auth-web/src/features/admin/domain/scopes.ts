/** OIDC scopes supported by the authorization server. */
export const OIDC_SCOPES = [
  {
    id: "openid",
    label: "OpenID",
    description: "Required for OIDC. Enables identity verification.",
    required: true,
  },
  {
    id: "profile",
    label: "Profile",
    description: "Access to name and related profile claims.",
    required: false,
  },
  {
    id: "email",
    label: "Email",
    description: "Access to email address claims.",
    required: false,
  },
  {
    id: "offline_access",
    label: "Offline access",
    description: "Issue refresh tokens for long-lived sessions.",
    required: false,
  },
] as const;

export type OidcScopeId = (typeof OIDC_SCOPES)[number]["id"];

export const DEFAULT_OAUTH_SCOPES: readonly OidcScopeId[] = ["openid", "profile", "email"];

export const DEFAULT_GRANT_TYPES = ["authorization_code", "refresh_token"] as const;
export const DEFAULT_RESPONSE_TYPES = ["code"] as const;

/** Ensures openid is always included when at least one scope is selected. */
export function normalizeSelectedScopes(scopes: readonly string[]): string[] {
  const unique = [
    ...new Set(scopes.map((scope) => scope.trim()).filter((scope) => scope.length > 0)),
  ];
  if (unique.length === 0) {
    return ["openid"];
  }
  if (!unique.includes("openid")) {
    return ["openid", ...unique];
  }
  return unique;
}

/** Validates that every scope is one the server recognizes. */
export function isKnownScope(scope: string): scope is OidcScopeId {
  return OIDC_SCOPES.some((entry) => entry.id === scope);
}
