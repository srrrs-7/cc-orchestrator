import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { createClient, createUser } from "../api/client";
import type {
  CreateClientRequest,
  CreateClientResponse,
  CreateUserRequest,
  CreateUserResponse,
} from "../api/schema";
import { adminClientsQueryKey, adminUsersQueryKey } from "./useAdminQueries";

export function useCreateUser() {
  const queryClient = useQueryClient();
  return useMutation<CreateUserResponse, ApiError, CreateUserRequest>({
    mutationFn: createUser,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminUsersQueryKey });
    },
  });
}

export function useCreateClient() {
  const queryClient = useQueryClient();
  return useMutation<CreateClientResponse, ApiError, CreateClientRequest>({
    mutationFn: createClient,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminClientsQueryKey });
    },
  });
}
