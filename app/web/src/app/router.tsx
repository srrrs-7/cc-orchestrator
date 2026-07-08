import { createRootRoute, createRoute, createRouter, useParams } from "@tanstack/react-router";
import { z } from "zod";
import { CreateTaskForm } from "../features/tasks/components/CreateTaskForm";
import { TaskFilters } from "../features/tasks/components/TaskFilters";
import { TaskItem } from "../features/tasks/components/TaskItem";
import { TaskList } from "../features/tasks/components/TaskList";
import { TaskSummary } from "../features/tasks/components/TaskSummary";
import { TASK_STATUSES } from "../features/tasks/domain/task";
import { useTaskQuery } from "../features/tasks/hooks/useTasks";
import { App } from "./App";

// The `status` search param on `/` is external data (comes from the
// URL), so it is validated with zod before being typed. `.catch`
// falls back to "all" instead of throwing on an unrecognized value,
// since a malformed URL shouldn't crash the route.
const taskListSearchSchema = z.object({
  status: z.enum(["all", ...TASK_STATUSES]).catch("all"),
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
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold">{data.title}</h1>
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
