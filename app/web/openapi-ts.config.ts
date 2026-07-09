import { defineConfig } from "@hey-api/openapi-ts";

/**
 * Generates typed SDK functions, Zod schemas, and TanStack Query options
 * from the Go API's OpenAPI contract (app/api/docs/openapi.yaml), which is
 * the contract of record for the tasks feature (SPEC-003). Run with
 * `bun run generate`; output is committed under
 * src/features/tasks/api/generated (see biome.json for the lint/format
 * exclusion — typecheck and build still cover it).
 */
export default defineConfig({
  input: "../api/docs/openapi.yaml",
  output: "src/features/tasks/api/generated",
  plugins: [
    "@hey-api/client-fetch",
    "@hey-api/typescript",
    {
      name: "zod",
      // Pin explicitly: the web app is on zod v4 (see package.json).
      compatibilityVersion: 4,
    },
    "@tanstack/react-query",
  ],
});
