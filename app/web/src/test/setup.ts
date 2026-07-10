import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll } from "vitest";
import { server } from "./msw-server";

// The import above extends vitest's `expect` with jest-dom matchers
// (toBeDisabled, toHaveTextContent, ...) and augments vitest's
// `Assertion` type accordingly, at both runtime and type-check time.

beforeAll(() => {
  server.listen({ onUnhandledRequest: "error" });
});

afterEach(() => {
  cleanup();
  server.resetHandlers();
});

afterAll(() => {
  server.close();
});
