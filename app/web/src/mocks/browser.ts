import { setupWorker } from "msw/browser";
import { handlers, oidcHandlers } from "./handlers";

export const worker = setupWorker(...oidcHandlers, ...handlers);
