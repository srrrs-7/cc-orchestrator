import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Button } from "../../../shared/ui/Button";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { Input } from "../../../shared/ui/Input";
import { Select } from "../../../shared/ui/Select";
import type { CreateTaskRequest } from "../api/schema";
import { createTaskRequestSchema } from "../api/schema";
import { TASK_PRIORITIES } from "../domain/task";
import { useCreateTask } from "../hooks/useTasks";

/** Form for creating a new task. Validation lives in the zod schema
 * that also describes the create-task API request (single source of
 * truth for "what a valid new task looks like"). */
export function CreateTaskForm() {
  const createTask = useCreateTask();
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<CreateTaskRequest>({
    resolver: zodResolver(createTaskRequestSchema),
    defaultValues: { title: "", priority: "medium" },
  });

  const onSubmit = handleSubmit((values) => {
    createTask.mutate(values, { onSuccess: () => reset() });
  });

  return (
    <Card className="p-4 sm:p-5">
      <CardHeader
        title="New task"
        description="Add a task to your list. High-priority items appear first."
      />
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <label htmlFor="title" className="text-sm font-medium text-gray-700">
            Title
          </label>
          <Input
            id="title"
            type="text"
            placeholder="What needs to be done?"
            aria-invalid={errors.title !== undefined}
            {...register("title")}
          />
          {errors.title ? (
            <p role="alert" className="text-xs text-red-600">
              {errors.title.message}
            </p>
          ) : null}
        </div>
        <div className="flex flex-col gap-1 sm:max-w-xs">
          <label htmlFor="priority" className="text-sm font-medium text-gray-700">
            Priority
          </label>
          <Select id="priority" {...register("priority")}>
            {TASK_PRIORITIES.map((priority) => (
              <option key={priority} value={priority}>
                {priority.charAt(0).toUpperCase() + priority.slice(1)}
              </option>
            ))}
          </Select>
        </div>
        <Button
          type="submit"
          disabled={isSubmitting || createTask.isPending}
          className="w-full sm:w-auto sm:self-start"
        >
          {createTask.isPending ? "Adding…" : "Add task"}
        </Button>
      </form>
    </Card>
  );
}
