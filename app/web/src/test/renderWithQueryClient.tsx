import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RenderOptions } from "@testing-library/react";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";

/**
 * A fresh QueryClient per test (retries disabled so failing requests
 * surface immediately instead of slowing tests down / racing with
 * `waitFor` timeouts). Never share the app's production singleton
 * (src/lib/queryClient.ts) between tests: it would leak cached
 * server state across test cases.
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

/** Renders `ui` wrapped in a QueryClientProvider backed by a fresh QueryClient. */
export function renderWithQueryClient(ui: ReactElement, options?: RenderOptions) {
  const queryClient = createTestQueryClient();
  const utils = render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
    options,
  );
  return { queryClient, ...utils };
}
