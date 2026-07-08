// Public surface of the "tasks" feature. Other parts of the app
// should import from here rather than reaching into feature
// internals directly.

export { CreateTaskForm } from "./components/CreateTaskForm";
export { TaskFilters } from "./components/TaskFilters";
export { TaskItem } from "./components/TaskItem";
export { TaskList } from "./components/TaskList";
export { TaskSummary } from "./components/TaskSummary";
export { InvalidTransitionError } from "./domain/errors";
export type { Task, TaskPriority, TaskStatus, TaskStatusSummary } from "./domain/task";
export {
  canComplete,
  canStart,
  completeTask,
  filterByStatus,
  sortByPriority,
  startTask,
  summarize,
  TASK_PRIORITIES,
  TASK_STATUSES,
} from "./domain/task";

export { useCreateTask, useTasksQuery, useUpdateTaskStatus } from "./hooks/useTasks";
