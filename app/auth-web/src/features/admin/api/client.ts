import { ZodError } from "zod";
import type { z } from "zod";
import { ApiError } from "../../../shared/api/errors";
import { getStoredAdminApiKey } from "../domain/credentials";
import {
  type AdminClient,
  type AdminErrorResponse,
  type AdminUser,
  adminClientSchema,
  adminErrorResponseSchema,
  adminUserSchema,
  type CreateClientRequest,
  type CreateClientResponse,
  createClientResponseSchema,
  type CreateUserRequest,
  type CreateUserResponse,
  createUserResponseSchema,
  listClientsResponseSchema,
  listUsersResponseSchema,
  type ListClientsResponse,
  type ListUsersResponse,
  type UpdateClientRequest,
  type UpdateUserRequest,
} from "./schema";

const rawBaseUrl = import.meta.env.VITE_AUTH_BASE_URL;
const DEFAULT_BASE_PATH = "/auth";

function resolveBaseUrl(): string {
  if (typeof rawBaseUrl === "string" && rawBaseUrl.length > 0) {
    return rawBaseUrl.replace(/\/$/, "");
  }
  return `${window.location.origin}${DEFAULT_BASE_PATH}`;
}

function parseResponse<T>(schema: z.ZodType<T>, data: unknown, status: number): T {
  try {
    return schema.parse(data);
  } catch (cause) {
    if (cause instanceof ZodError) {
      throw new ApiError("Received an unexpected response shape from the server", {
        status,
        cause,
      });
    }
    throw cause;
  }
}

async function readAdminError(response: Response): Promise<AdminErrorResponse | null> {
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return null;
  }
  try {
    const body: unknown = await response.json();
    return adminErrorResponseSchema.parse(body);
  } catch {
    return null;
  }
}

async function adminRequest(path: string, init: RequestInit): Promise<Response> {
  const apiKey = getStoredAdminApiKey();
  if (apiKey === null) {
    throw new ApiError("Admin API key is not configured", { status: 401 });
  }

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${apiKey}`);
  if (init.body !== undefined && init.body !== null && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  try {
    return await fetch(`${resolveBaseUrl()}${path}`, {
      ...init,
      headers,
    });
  } catch (cause) {
    throw new ApiError("Network request failed", { status: 0, cause });
  }
}

async function readAdminFailure(response: Response): Promise<never> {
  const errorBody = await readAdminError(response);
  const message =
    errorBody?.error_description ??
    errorBody?.error ??
    `Request failed with status ${response.status}`;
  throw new ApiError(message, { status: response.status });
}

async function adminFetch<T>(path: string, init: RequestInit, schema: z.ZodType<T>): Promise<T> {
  const response = await adminRequest(path, init);
  if (!response.ok) {
    return readAdminFailure(response);
  }
  const data: unknown = await response.json();
  return parseResponse(schema, data, response.status);
}

async function adminFetchVoid(path: string, init: RequestInit): Promise<void> {
  const response = await adminRequest(path, init);
  if (!response.ok) {
    return readAdminFailure(response);
  }
}

export async function createUser(input: CreateUserRequest): Promise<CreateUserResponse> {
  return adminFetch(
    "/admin/users",
    { method: "POST", body: JSON.stringify(input) },
    createUserResponseSchema,
  );
}

export async function updateUser(userId: string, input: UpdateUserRequest): Promise<AdminUser> {
  const body = {
    username: input.username,
    name: input.name,
    email: input.email,
    ...(input.password !== undefined && input.password.length > 0
      ? { password: input.password }
      : {}),
  };
  return adminFetch(
    `/admin/users/${encodeURIComponent(userId)}`,
    {
      method: "PUT",
      body: JSON.stringify(body),
    },
    adminUserSchema,
  );
}

export async function deleteUser(userId: string): Promise<void> {
  return adminFetchVoid(`/admin/users/${encodeURIComponent(userId)}`, { method: "DELETE" });
}

export async function createClient(input: CreateClientRequest): Promise<CreateClientResponse> {
  const body = {
    client_id: input.client_id,
    redirect_uris: input.redirect_uris,
    allowed_scopes: input.allowed_scopes,
    response_types: input.response_types,
    grant_types: input.grant_types,
    ...(input.client_secret.length > 0 ? { client_secret: input.client_secret } : {}),
  };

  return adminFetch(
    "/admin/clients",
    { method: "POST", body: JSON.stringify(body) },
    createClientResponseSchema,
  );
}

export async function updateClient(
  clientId: string,
  input: UpdateClientRequest,
): Promise<AdminClient> {
  const body = {
    redirect_uris: input.redirect_uris,
    allowed_scopes: input.allowed_scopes,
    response_types: input.response_types,
    grant_types: input.grant_types,
    ...(input.client_secret.length > 0 ? { client_secret: input.client_secret } : {}),
  };
  return adminFetch(
    `/admin/clients/${encodeURIComponent(clientId)}`,
    {
      method: "PUT",
      body: JSON.stringify(body),
    },
    adminClientSchema,
  );
}

export async function deleteClient(clientId: string): Promise<void> {
  return adminFetchVoid(`/admin/clients/${encodeURIComponent(clientId)}`, { method: "DELETE" });
}

export async function fetchUsers(): Promise<ListUsersResponse> {
  return adminFetch("/admin/users", { method: "GET" }, listUsersResponseSchema);
}

export async function fetchUserById(userId: string): Promise<AdminUser> {
  return adminFetch(
    `/admin/users/${encodeURIComponent(userId)}`,
    { method: "GET" },
    adminUserSchema,
  );
}

export async function fetchClients(): Promise<ListClientsResponse> {
  return adminFetch("/admin/clients", { method: "GET" }, listClientsResponseSchema);
}

export async function fetchClientById(clientId: string): Promise<AdminClient> {
  return adminFetch(
    `/admin/clients/${encodeURIComponent(clientId)}`,
    { method: "GET" },
    adminClientSchema,
  );
}
