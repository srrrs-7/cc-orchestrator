import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import {
  createClient,
  createUser,
  deleteClient,
  deleteUser,
  updateClient,
  updateUser,
} from "../api/client";
import type {
  AdminClient,
  AdminUser,
  CreateClientRequest,
  CreateClientResponse,
  CreateUserRequest,
  CreateUserResponse,
  UpdateClientRequest,
  UpdateUserRequest,
} from "../api/schema";
import {
  adminClientsQueryKey,
  adminUsersQueryKey,
  clientQueryKey,
  userQueryKey,
} from "./useAdminQueries";

export function useCreateUser() {
  const queryClient = useQueryClient();
  return useMutation<CreateUserResponse, ApiError, CreateUserRequest>({
    mutationFn: createUser,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminUsersQueryKey });
    },
  });
}

export function useUpdateUser(userId: string) {
  const queryClient = useQueryClient();
  return useMutation<AdminUser, ApiError, UpdateUserRequest>({
    mutationFn: (input) => updateUser(userId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminUsersQueryKey });
      void queryClient.invalidateQueries({ queryKey: userQueryKey(userId) });
    },
  });
}

export function useDeleteUser() {
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: deleteUser,
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

export function useUpdateClient(clientId: string) {
  const queryClient = useQueryClient();
  return useMutation<AdminClient, ApiError, UpdateClientRequest>({
    mutationFn: (input) => updateClient(clientId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminClientsQueryKey });
      void queryClient.invalidateQueries({ queryKey: clientQueryKey(clientId) });
    },
  });
}

export function useDeleteClient() {
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: deleteClient,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminClientsQueryKey });
    },
  });
}
