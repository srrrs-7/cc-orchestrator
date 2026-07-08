import { createRootRoute, createRoute, createRouter, useParams } from "@tanstack/react-router";
import { z } from "zod";
import { CreateTaskForm } from "../features/tasks/components/CreateTaskForm";
import { TaskFilters } from "../features/tasks/components/TaskFilters";
import { TaskItem } from "../features/tasks/components/TaskItem";
import { TaskList } from "../features/tasks/components/TaskList";
import { TaskSummary } from "../features/tasks/components/TaskSummary";
import { TASK_STATUSES } from "../features/tasks/domain/task";
import { useTasksQuery } from "../features/tasks/hooks/useTasks";
import { App } from "./App";

// The `status` search param on `/` is external data (comes from the
// URL), so it is validated with zod before being typed. `.catch`
// falls back to "all" instead of throwing on an unrecognized value,
// since a malformed URL shouldn't crash the route.
const taskListSearchSchema = z.object({
  status: z.enum(["all", ...TASK_STATUSES]).catch("all"),
});

type TaskListSearch = z.infer<typeof taskListSearchSchema>;

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
  const { data, isLoading, isError, error } = useTasksQuery();

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

  const task = (data ?? []).find((candidate) => candidate.id === taskId);

  if (task === undefined) {
    return <p className="text-sm text-gray-500">Task not found.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold">{task.title}</h1>
      <TaskItem task={task} />
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
  component: TaskDetailPage,
});

const routeTree = rootRoute.addChildren([indexRoute, taskDetailRoute]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
