import { createContext, useCallback, useContext, useState } from "react";
import type { ReactNode } from "react";
import { buildAuthorizeUrl, exchangeCode, fetchDiscovery } from "../api/oidc";
import { resolveAuthConfig } from "../domain/config";
import { deriveCodeChallenge, generateCodeVerifier } from "../domain/pkce";
import {
  clearAllAuthStorage,
  clearStoredPkce,
  getStoredPkce,
  getStoredSession,
  isSessionValid,
  setReturnTo,
  setStoredPkce,
  setStoredSession,
} from "../domain/session";
import { extractDisplayName, extractSub, parseJwtPayload } from "../domain/token";

export type AuthUser = {
  readonly sub: string;
  readonly displayName: string;
};

type AuthContextValue = {
  readonly isAuthenticated: boolean;
  readonly user: AuthUser | null;
  /**
   * Initiate the OIDC Authorization Code + PKCE login flow.
   * Stores PKCE state, then redirects the browser to the authorization
   * endpoint. `returnTo` is the path to navigate to after the callback
   * completes (defaults to "/").
   */
  readonly login: (returnTo?: string) => Promise<void>;
  /** Clear the session and redirect to /login. */
  readonly logout: () => void;
  /**
   * Complete the OIDC callback: validate `state`, exchange `code` for tokens,
   * parse the ID token, and persist the session.
   * Call this from the /callback route component.
   */
  readonly handleCallback: (code: string, state: string) => Promise<void>;
  /** Return the current access token, or null if the session has expired. */
  readonly getAccessToken: () => string | null;
};

const AuthContext = createContext<AuthContextValue | null>(null);

type AuthProviderProps = {
  readonly children: ReactNode;
};

export function AuthProvider({ children }: AuthProviderProps) {
  // Initialise from any existing session in sessionStorage so that a page
  // reload keeps the user logged in for the duration of the browser tab.
  const [user, setUser] = useState<AuthUser | null>(() => {
    const session = getStoredSession();
    if (!session || !isSessionValid(session)) return null;
    return { sub: session.sub, displayName: session.displayName };
  });

  const login = useCallback(async (returnTo = "/"): Promise<void> => {
    const config = resolveAuthConfig();
    const verifier = await generateCodeVerifier();
    const challenge = await deriveCodeChallenge(verifier);
    const state = crypto.randomUUID();

    setStoredPkce({ verifier, state });
    setReturnTo(returnTo);

    const discovery = await fetchDiscovery(config.issuer);
    const authorizeUrl = buildAuthorizeUrl({
      authorizationEndpoint: discovery.authorization_endpoint,
      clientId: config.clientId,
      redirectUri: config.redirectUri,
      scopes: config.scopes,
      codeChallenge: challenge,
      state,
    });

    window.location.href = authorizeUrl;
  }, []);

  const logout = useCallback((): void => {
    clearAllAuthStorage();
    setUser(null);
    window.location.href = "/login";
  }, []);

  const handleCallback = useCallback(async (code: string, state: string): Promise<void> => {
    const pkce = getStoredPkce();
    if (!pkce) {
      throw new Error("No PKCE state found — please restart the sign-in flow.");
    }
    if (pkce.state !== state) {
      throw new Error("State mismatch — possible CSRF. Please restart the sign-in flow.");
    }
    clearStoredPkce();

    const config = resolveAuthConfig();
    const discovery = await fetchDiscovery(config.issuer);
    const tokens = await exchangeCode({
      tokenEndpoint: discovery.token_endpoint,
      code,
      redirectUri: config.redirectUri,
      clientId: config.clientId,
      codeVerifier: pkce.verifier,
    });

    const payload = parseJwtPayload(tokens.id_token);
    const sub = extractSub(payload);
    const displayName = extractDisplayName(payload);

    setStoredSession({
      accessToken: tokens.access_token,
      idToken: tokens.id_token,
      refreshToken: tokens.refresh_token,
      expiresAt: Date.now() + tokens.expires_in * 1000,
      sub,
      displayName,
    });

    setUser({ sub, displayName });
  }, []);

  const getAccessToken = useCallback((): string | null => {
    const session = getStoredSession();
    if (!session || !isSessionValid(session)) return null;
    return session.accessToken;
  }, []);

  const value: AuthContextValue = {
    isAuthenticated: user !== null,
    user,
    login,
    logout,
    handleCallback,
    getAccessToken,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (ctx === null) {
    throw new Error("useAuth must be used inside <AuthProvider>");
  }
  return ctx;
}
