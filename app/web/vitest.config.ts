import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// Separate from vite.config.ts (the production build config) so that
// test-only concerns (environment, setup files, coverage) never touch
// the build pipeline. Plugins are duplicated here only because Vitest
// needs the React JSX transform / Tailwind resolution for the same
// source tree it type-checks and renders in jsdom.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    restoreMocks: true,
  },
});
