import { useQuery } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { fetchClients, fetchUsers } from "../api/client";
import type { AdminClient, AdminUser } from "../api/schema";

export const adminUsersQueryKey = ["admin", "users"] as const;
export const adminClientsQueryKey = ["admin", "clients"] as const;

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
