const ADMIN_API_KEY_STORAGE_KEY = "auth-admin-api-key";

export function getStoredAdminApiKey(): string | null {
  try {
    const value = sessionStorage.getItem(ADMIN_API_KEY_STORAGE_KEY);
    return value === null || value.length === 0 ? null : value;
  } catch {
    return null;
  }
}

export function setStoredAdminApiKey(apiKey: string): void {
  sessionStorage.setItem(ADMIN_API_KEY_STORAGE_KEY, apiKey);
}

export function clearStoredAdminApiKey(): void {
  sessionStorage.removeItem(ADMIN_API_KEY_STORAGE_KEY);
}

export function hasStoredAdminApiKey(): boolean {
  return getStoredAdminApiKey() !== null;
}
