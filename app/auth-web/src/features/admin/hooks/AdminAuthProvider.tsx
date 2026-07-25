import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import {
  clearStoredAdminApiKey,
  getStoredAdminApiKey,
  setStoredAdminApiKey,
} from "../domain/credentials";

type AdminAuthContextValue = {
  readonly apiKey: string | null;
  readonly isConfigured: boolean;
  readonly setApiKey: (apiKey: string) => void;
  readonly clearApiKey: () => void;
};

const AdminAuthContext = createContext<AdminAuthContextValue | null>(null);

type AdminAuthProviderProps = {
  readonly children: ReactNode;
};

export function AdminAuthProvider({ children }: AdminAuthProviderProps) {
  const [apiKey, setApiKeyState] = useState<string | null>(() => getStoredAdminApiKey());

  const setApiKey = useCallback((nextKey: string) => {
    setStoredAdminApiKey(nextKey);
    setApiKeyState(nextKey);
  }, []);

  const clearApiKey = useCallback(() => {
    clearStoredAdminApiKey();
    setApiKeyState(null);
  }, []);

  const value = useMemo<AdminAuthContextValue>(
    () => ({
      apiKey,
      isConfigured: apiKey !== null && apiKey.length > 0,
      setApiKey,
      clearApiKey,
    }),
    [apiKey, setApiKey, clearApiKey],
  );

  return <AdminAuthContext.Provider value={value}>{children}</AdminAuthContext.Provider>;
}

export function useAdminAuth(): AdminAuthContextValue {
  const context = useContext(AdminAuthContext);
  if (context === null) {
    throw new Error("useAdminAuth must be used within AdminAuthProvider");
  }
  return context;
}
