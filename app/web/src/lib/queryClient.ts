import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      // Avoid an immediate refetch when navigating between the list and
      // detail views for the same data within a short window.
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
  },
});
