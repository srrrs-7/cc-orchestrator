import { defineConfig } from "@hey-api/openapi-ts";

/**
 * Generates typed SDK functions, Zod schemas, and TanStack Query options
 * from the Go API's OpenAPI contract (app/api/docs/openapi.yaml), which is
 * the contract of record for the tasks feature (SPEC-003). Run with
 * `bun run generate`; output is committed under
 * src/features/tasks/api/generated (see biome.json for the lint/format
 * exclusion — typecheck and build still cover it).
 *
 * `@hey-api/openapi-ts` is pinned in package.json to a `0.0.0-next-*`
 * prerelease, not the latest stable (0.99.0 at time of writing). Every
 * published release through 0.99.0 crashes under the TypeScript 7 native
 * compiler (`typescript@7.0.2`, see package.json) with
 * `TypeError: undefined is not an object (evaluating
 * 'ts.SyntaxKind.AnyKeyword')` inside the tool's own bundled code, because
 * it hasn't shipped a stable release with TS7 support yet. The pinned
 * `next` build was verified to run under TS7 and produce output
 * equivalent (modulo indentation-only formatting) to the previously
 * committed generated files. Bump back to a stable release once one
 * supports TS7 (do not revert `typescript` to <7 to work around this —
 * SPEC-007 requires TS7). The pin was locked into bun.lock via the
 * lock-then-restore process in .claude/rules/web.md (temporarily add
 * `@hey-api/openapi-ts` / `@hey-api/codegen-cli` to
 * `minimumReleaseAgeExcludes` in bunfig.toml, `bun install`, then remove
 * them again — already-locked versions pass the release-age gate).
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
