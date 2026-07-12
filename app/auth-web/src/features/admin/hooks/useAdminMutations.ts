import { useMutation } from "@tanstack/react-query";
import type { ApiError } from "../../../shared/api/errors";
import { createClient, createUser } from "../api/client";
import type {
  CreateClientRequest,
  CreateClientResponse,
  CreateUserRequest,
  CreateUserResponse,
} from "../api/schema";

export function useCreateUser() {
  return useMutation<CreateUserResponse, ApiError, CreateUserRequest>({
    mutationFn: createUser,
  });
}

export function useCreateClient() {
  return useMutation<CreateClientResponse, ApiError, CreateClientRequest>({
    mutationFn: createClient,
  });
}
