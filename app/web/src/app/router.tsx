import { createRootRoute, createRoute, createRouter, useParams } from "@tanstack/react-router";
import { z } from "zod";
import { CreateTaskForm } from "../features/tasks/components/CreateTaskForm";
import { TaskFilters } from "../features/tasks/components/TaskFilters";
import { TaskItem } from "../features/tasks/components/TaskItem";
import { TaskList } from "../features/tasks/components/TaskList";
import { TaskSummary } from "../features/tasks/components/TaskSummary";
import { DEFAULT_LIMIT, MAX_LIMIT } from "../features/tasks/domain/pagination";
import { TASK_STATUSES } from "../features/tasks/domain/task";
import { useTaskQuery } from "../features/tasks/hooks/useTasks";
import { App } from "./App";

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

const rootRoute = createRootRoute({
  component: App,
});

function TaskListPage() {
  return (
    <div className="flex flex-col gap-6">
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
    return <p className="text-sm text-gray-500">Loading task...</p>;
  }

  if (isError) {
    return (
      <p role="alert" className="text-sm text-red-600">
        Failed to load task: {error.message}
      </p>
    );
  }

  if (data === undefined) {
    return <p className="text-sm text-gray-500">Task not found.</p>;
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <h1 className="break-words text-xl font-semibold">{data.title}</h1>
      <TaskItem task={data} />
    </div>
  );
}

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  validateSearch: (search: Record<string, unknown>): TaskListSearch =>
    taskListSearchSchema.parse(search),
  component: TaskListPage,
});

const taskDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tasks/$taskId",
  params: {
    parse: (rawParams) => taskDetailParamsSchema.parse(rawParams),
  },
  component: TaskDetailPage,
});

const routeTree = rootRoute.addChildren([indexRoute, taskDetailRoute]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
