import { ZodError } from "zod";
import type { z } from "zod";
import { ApiError } from "../../../shared/api/errors";
import { getStoredAdminApiKey } from "../domain/credentials";
import {
  type AdminErrorResponse,
  adminErrorResponseSchema,
  type CreateClientRequest,
  type CreateClientResponse,
  createClientResponseSchema,
  type CreateUserRequest,
  type CreateUserResponse,
  createUserResponseSchema,
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

async function adminFetch<T>(path: string, init: RequestInit, schema: z.ZodType<T>): Promise<T> {
  const apiKey = getStoredAdminApiKey();
  if (apiKey === null) {
    throw new ApiError("Admin API key is not configured", { status: 401 });
  }

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${apiKey}`);
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  let response: Response;
  try {
    response = await fetch(`${resolveBaseUrl()}${path}`, {
      ...init,
      headers,
    });
  } catch (cause) {
    throw new ApiError("Network request failed", { status: 0, cause });
  }

  if (!response.ok) {
    const errorBody = await readAdminError(response);
    const message =
      errorBody?.error_description ??
      errorBody?.error ??
      `Request failed with status ${response.status}`;
    throw new ApiError(message, { status: response.status });
  }

  const data: unknown = await response.json();
  return parseResponse(schema, data, response.status);
}

export async function createUser(input: CreateUserRequest): Promise<CreateUserResponse> {
  return adminFetch(
    "/admin/users",
    {
      method: "POST",
      body: JSON.stringify(input),
    },
    createUserResponseSchema,
  );
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
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    createClientResponseSchema,
  );
}
