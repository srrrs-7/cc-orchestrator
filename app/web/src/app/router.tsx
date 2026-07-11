import {
  Link,
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
  useNavigate,
  useParams,
  useSearch,
} from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { z } from "zod";
import { useAuth } from "../features/auth/hooks/AuthProvider";
import { LoginButton } from "../features/auth/components/LoginButton";
import {
  isAuthenticated,
  setReturnTo,
  clearReturnTo,
  getReturnTo,
} from "../features/auth/domain/session";
import { Alert } from "../shared/ui/Alert";
import { CreateTaskForm } from "../features/tasks/components/CreateTaskForm";
import { TaskDetailSkeleton } from "../features/tasks/components/TaskDetailSkeleton";
import { TaskFilters } from "../features/tasks/components/TaskFilters";
import { TaskItem } from "../features/tasks/components/TaskItem";
import { TaskList } from "../features/tasks/components/TaskList";
import { TaskSummary } from "../features/tasks/components/TaskSummary";
import { DEFAULT_LIMIT, MAX_LIMIT } from "../features/tasks/domain/pagination";
import { TASK_STATUSES } from "../features/tasks/domain/task";
import { useTaskQuery } from "../features/tasks/hooks/useTasks";
import { App } from "./App";

// ---------------------------------------------------------------------------
// Task list / detail schemas (unchanged)
// ---------------------------------------------------------------------------

// The `status`/`limit`/`offset` search params on `/` are external data
// (come from the URL), so they are validated with zod before being
// typed. `.catch` falls back to a sensible default instead of throwing
// on a malformed/missing value, since a malformed URL shouldn't crash
// the route. `limit`/`offset` (SPEC-008) mirror the defaults/bounds the
// server applies (domain/pagination.ts) so an out-of-range value in the
// URL still produces a request the server accepts, but the server's
// echoed response (not this schema) is the source of truth once data
// has loaded.
const taskListSearchSchema = z.object({
  status: z.enum(["all", ...TASK_STATUSES]).catch("all"),
  limit: z.coerce.number().int().min(1).max(MAX_LIMIT).catch(DEFAULT_LIMIT),
  offset: z.coerce.number().int().min(0).catch(0),
});

type TaskListSearch = z.infer<typeof taskListSearchSchema>;

// The `taskId` path param on `/tasks/$taskId` is external data too
// (it comes from the URL), so it is validated with zod before being
// typed, the same way the `status` search param above is. Unlike the
// search param, an invalid path param has no sensible fallback, so it
// throws (TanStack Router wraps this into a PathParamError for the
// route match, mirroring how validateSearch throwing becomes a
// SearchParamError).
const taskDetailParamsSchema = z.object({
  taskId: z.string().min(1),
});

// ---------------------------------------------------------------------------
// Root route
// ---------------------------------------------------------------------------

const rootRoute = createRootRoute({
  component: App,
});

// ---------------------------------------------------------------------------
// Page components
// ---------------------------------------------------------------------------

function TaskListPage() {
  return (
    <div className="flex flex-col gap-6 sm:gap-8">
      <TaskSummary />
      <CreateTaskForm />
      <TaskFilters />
      <TaskList />
    </div>
  );
}

function TaskDetailPage() {
  const { taskId } = useParams({ from: "/tasks/$taskId" });
  const { data, isLoading, isError, error } = useTaskQuery(taskId);

  if (isLoading) {
    return <TaskDetailSkeleton />;
  }

  if (isError) {
    return <Alert>Failed to load task: {error.message}</Alert>;
  }

  if (data === undefined) {
    return (
      <div className="rounded-lg border border-dashed border-gray-300 bg-surface-muted px-4 py-10 text-center">
        <p className="text-base font-medium text-gray-900">Task not found</p>
        <p className="mt-1 text-sm text-gray-500">
          The task may have been removed or the link is invalid.
        </p>
        <Link
          to="/"
          search={{ status: "all", limit: DEFAULT_LIMIT, offset: 0 }}
          className="mt-4 inline-flex text-sm font-medium text-accent hover:text-accent-hover focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 pointer-coarse:min-h-11 pointer-coarse:items-center"
        >
          ← Back to task list
        </Link>
      </div>
    );
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <Link
        to="/"
        search={{ status: "all", limit: DEFAULT_LIMIT, offset: 0 }}
        className="inline-flex w-fit text-sm font-medium text-gray-600 hover:text-accent focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 pointer-coarse:min-h-11 pointer-coarse:items-center"
      >
        ← Back to task list
      </Link>
      <h1 className="break-words text-xl font-semibold text-gray-900 sm:text-2xl">{data.title}</h1>
      <TaskItem task={data} showTimestamps />
    </div>
  );
}

/**
 * Standalone login page shown when an unauthenticated user tries to access
 * a protected route. Wraps `LoginButton` in a centered card.
 */
function LoginPage() {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-6 px-4 text-center">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold text-gray-900">Sign in to Task Manager</h1>
        <p className="text-sm text-gray-500">You need to sign in to view and manage your tasks.</p>
      </div>
      <LoginButton />
    </div>
  );
}

// Search params accepted on the /callback route.
const callbackSearchSchema = z.object({
  // Present on successful OIDC redirects.
  code: z.string().optional().catch(undefined),
  state: z.string().optional().catch(undefined),
  // Present when the auth server reports an error.
  error: z.string().optional().catch(undefined),
  error_description: z.string().optional().catch(undefined),
});

type CallbackSearch = z.infer<typeof callbackSearchSchema>;

/**
 * OAuth callback page. Exchanges the authorization code for tokens and
 * redirects to the stored return-to path (or "/" as fallback).
 *
 * StrictMode runs effects twice in development. The second invocation sees
 * that a valid session already exists (set by the first) and bails out
 * immediately to avoid a double code exchange.
 */
function CallbackPage() {
  const search = useSearch({ from: "/callback" });
  const { handleCallback } = useAuth();
  const navigate = useNavigate();
  const [callbackError, setCallbackError] = useState<string | null>(null);

  useEffect(() => {
    // Already authenticated (e.g., StrictMode second run after first succeeded).
    if (isAuthenticated()) {
      void navigate({ to: "/", search: { status: "all", limit: DEFAULT_LIMIT, offset: 0 } });
      return;
    }

    if (search.error) {
      setCallbackError(search.error_description ?? search.error ?? "Authorization failed");
      return;
    }

    const code = search.code;
    const state = search.state;

    if (!code || !state) {
      setCallbackError("Missing code or state in callback URL");
      return;
    }

    handleCallback(code, state)
      .then(() => {
        const returnTo = getReturnTo();
        clearReturnTo();
        // Use replace so the /callback URL doesn't remain in the browser history.
        window.location.replace(returnTo);
      })
      .catch((err: unknown) => {
        setCallbackError(err instanceof Error ? err.message : "Authentication failed");
      });
  }, [search.code, search.state, search.error, search.error_description, handleCallback, navigate]);

  if (callbackError) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 px-4 text-center">
        <Alert>Sign-in failed: {callbackError}</Alert>
        <LoginButton />
      </div>
    );
  }

  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <p className="text-sm text-gray-500">Completing sign in…</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Route definitions
// ---------------------------------------------------------------------------

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  validateSearch: (search: Record<string, unknown>): TaskListSearch =>
    taskListSearchSchema.parse(search),
  beforeLoad: ({ location }) => {
    if (!isAuthenticated()) {
      setReturnTo(location.href);
      throw redirect({ to: "/login" });
    }
  },
  component: TaskListPage,
});

const taskDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tasks/$taskId",
  params: {
    parse: (rawParams) => taskDetailParamsSchema.parse(rawParams),
  },
  beforeLoad: ({ location }) => {
    if (!isAuthenticated()) {
      setReturnTo(location.href);
      throw redirect({ to: "/login" });
    }
  },
  component: TaskDetailPage,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  beforeLoad: () => {
    if (isAuthenticated()) {
      throw redirect({ to: "/", search: { status: "all", limit: DEFAULT_LIMIT, offset: 0 } });
    }
  },
  component: LoginPage,
});

const callbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/callback",
  validateSearch: (search: Record<string, unknown>): CallbackSearch =>
    callbackSearchSchema.parse(search),
  component: CallbackPage,
});

const routeTree = rootRoute.addChildren([indexRoute, taskDetailRoute, loginRoute, callbackRoute]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
