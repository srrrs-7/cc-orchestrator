import { z } from "zod";
import { normalizeSelectedScopes } from "../domain/scopes";
import { parseRedirectUris } from "../domain/redirectUris";

export const createUserRequestSchema = z.object({
  user_id: z.string().trim().min(1, "User ID is required"),
  username: z.string().trim().min(1, "Username is required"),
  password: z.string().min(8, "Password must be at least 8 characters"),
  name: z.string().trim().min(1, "Display name is required"),
  email: z.email("Enter a valid email address"),
});

export type CreateUserRequest = z.infer<typeof createUserRequestSchema>;

export const createUserResponseSchema = z.object({
  user_id: z.string(),
  username: z.string(),
});

export type CreateUserResponse = z.infer<typeof createUserResponseSchema>;

export const adminUserSchema = z.object({
  user_id: z.string(),
  username: z.string(),
  name: z.string(),
  email: z.string(),
});

export type AdminUser = z.infer<typeof adminUserSchema>;

export const listUsersResponseSchema = z.object({
  users: z.array(adminUserSchema),
});

export type ListUsersResponse = z.infer<typeof listUsersResponseSchema>;

export const createClientFormSchema = z
  .object({
    client_id: z.string().trim().min(1, "Client ID is required"),
    redirect_uris_text: z.string().trim().min(1, "At least one redirect URI is required"),
    allowed_scopes: z.array(z.string()).min(1, "Select at least one scope"),
    response_types: z.array(z.string()).min(1, "Select at least one response type"),
    grant_types: z.array(z.string()).min(1, "Select at least one grant type"),
    client_secret: z.string().optional(),
  })
  .refine((values) => parseRedirectUris(values.redirect_uris_text).length > 0, {
    message: "At least one redirect URI is required",
    path: ["redirect_uris_text"],
  });

export type CreateClientFormValues = z.infer<typeof createClientFormSchema>;

export type CreateClientRequest = {
  client_id: string;
  redirect_uris: string[];
  allowed_scopes: string[];
  response_types: string[];
  grant_types: string[];
  client_secret: string;
};

export function toCreateClientRequest(values: CreateClientFormValues): CreateClientRequest {
  return {
    client_id: values.client_id,
    redirect_uris: parseRedirectUris(values.redirect_uris_text),
    allowed_scopes: normalizeSelectedScopes(values.allowed_scopes),
    response_types: [...new Set(values.response_types)],
    grant_types: [...new Set(values.grant_types)],
    client_secret: values.client_secret?.trim() ?? "",
  };
}

export const createClientResponseSchema = z.object({
  client_id: z.string(),
  is_confidential: z.boolean(),
});

export type CreateClientResponse = z.infer<typeof createClientResponseSchema>;

export const adminClientSchema = z.object({
  client_id: z.string(),
  redirect_uris: z.array(z.string()),
  allowed_scopes: z.array(z.string()),
  response_types: z.array(z.string()),
  grant_types: z.array(z.string()),
  is_confidential: z.boolean(),
});

export type AdminClient = z.infer<typeof adminClientSchema>;

export const listClientsResponseSchema = z.object({
  clients: z.array(adminClientSchema),
});

export type ListClientsResponse = z.infer<typeof listClientsResponseSchema>;

export const adminErrorResponseSchema = z.object({
  error: z.string(),
  error_description: z.string().optional(),
});

export type AdminErrorResponse = z.infer<typeof adminErrorResponseSchema>;

export const adminApiKeySchema = z.object({
  apiKey: z.string().trim().min(1, "Admin API key is required"),
});

export type AdminApiKeyFormValues = z.infer<typeof adminApiKeySchema>;
