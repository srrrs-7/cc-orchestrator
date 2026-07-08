import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { AppProviders } from "./app/providers";
import { router } from "./app/router";
import "./index.css";

/**
 * Starts the MSW worker in development so the app is fully usable
 * without the Go API running. Tree-shaken out of production builds
 * because `import.meta.env.DEV` is statically replaced by Vite.
 */
async function enableMocking(): Promise<void> {
  if (!import.meta.env.DEV) return;
  const { worker } = await import("./mocks/browser");
  await worker.start({ onUnhandledRequest: "bypass" });
}

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("Root element not found");
}

enableMocking()
  .then(() => {
    createRoot(rootElement).render(
      <StrictMode>
        <AppProviders>
          <RouterProvider router={router} />
        </AppProviders>
      </StrictMode>,
    );
  })
  .catch((error: unknown) => {
    console.error("Failed to start the application", error);
  });
