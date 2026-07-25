import { describe, expect, it, beforeEach } from "vitest";
import {
  clearStoredAdminApiKey,
  getStoredAdminApiKey,
  hasStoredAdminApiKey,
  setStoredAdminApiKey,
} from "./credentials";

describe("credentials", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it("stores and reads the admin API key", () => {
    setStoredAdminApiKey("secret-key");
    expect(getStoredAdminApiKey()).toBe("secret-key");
    expect(hasStoredAdminApiKey()).toBe(true);
  });

  it("clears the stored key", () => {
    setStoredAdminApiKey("secret-key");
    clearStoredAdminApiKey();
    expect(getStoredAdminApiKey()).toBeNull();
    expect(hasStoredAdminApiKey()).toBe(false);
  });
});
