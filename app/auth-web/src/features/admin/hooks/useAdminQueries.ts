import { useQuery } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { fetchClientById, fetchClients, fetchUserById, fetchUsers } from "../api/client";
import type { AdminClient, AdminUser } from "../api/schema";

export const adminUsersQueryKey = ["admin", "users"] as const;
export const adminClientsQueryKey = ["admin", "clients"] as const;

export function userQueryKey(userId: string) {
  return [...adminUsersQueryKey, userId] as const;
}

export function clientQueryKey(clientId: string) {
  return [...adminClientsQueryKey, clientId] as const;
}

export function useUsersQuery(enabled = true) {
  return useQuery<AdminUser[], ApiError>({
    queryKey: adminUsersQueryKey,
    queryFn: async () => {
      const response = await fetchUsers();
      return response.users;
    },
    enabled,
  });
}

export function useUserQuery(userId: string, enabled = true) {
  return useQuery<AdminUser, ApiError>({
    queryKey: userQueryKey(userId),
    queryFn: () => fetchUserById(userId),
    enabled,
  });
}

export function useClientsQuery(enabled = true) {
  return useQuery<AdminClient[], ApiError>({
    queryKey: adminClientsQueryKey,
    queryFn: async () => {
      const response = await fetchClients();
      return response.clients;
    },
    enabled,
  });
}

export function useClientQuery(clientId: string, enabled = true) {
  return useQuery<AdminClient, ApiError>({
    queryKey: clientQueryKey(clientId),
    queryFn: () => fetchClientById(clientId),
    enabled,
  });
}
