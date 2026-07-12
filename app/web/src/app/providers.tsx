import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { AuthProvider } from "../features/auth/hooks/AuthProvider";
import { queryClient } from "../lib/queryClient";

type AppProvidersProps = {
  readonly children: ReactNode;
};

/** Wires up cross-cutting providers: AuthProvider → TanStack Query. */
export function AppProviders({ children }: AppProvidersProps) {
  return (
    <AuthProvider>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </AuthProvider>
  );
}
