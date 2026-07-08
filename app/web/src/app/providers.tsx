import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { queryClient } from "../lib/queryClient";

type AppProvidersProps = {
  readonly children: ReactNode;
};

/** Wires up cross-cutting providers (currently: TanStack Query). */
export function AppProviders({ children }: AppProvidersProps) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
