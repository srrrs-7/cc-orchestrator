import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { AdminAuthProvider } from "../features/admin/hooks/AdminAuthProvider";
import { queryClient } from "../lib/queryClient";

type AppProvidersProps = {
  readonly children: ReactNode;
};

export function AppProviders({ children }: AppProvidersProps) {
  return (
    <AdminAuthProvider>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </AdminAuthProvider>
  );
}
