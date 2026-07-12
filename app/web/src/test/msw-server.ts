import { setupServer } from "msw/node";
import { handlers, oidcHandlers } from "../mocks/handlers";

/**
 * Node-side MSW server for component/integration tests, sharing the
 * same request handlers as the browser worker (src/mocks/browser.ts)
 * used in development. Started/reset/stopped from src/test/setup.ts.
 */
export const server = setupServer(...oidcHandlers, ...handlers);
