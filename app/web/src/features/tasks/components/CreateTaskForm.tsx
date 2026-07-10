import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Button } from "../../../shared/ui/Button";
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
    <form
      onSubmit={onSubmit}
      className="flex flex-col gap-3 rounded border border-gray-200 bg-white p-4"
    >
      <div className="flex flex-col gap-1">
        <label htmlFor="title" className="text-sm font-medium">
          Title
        </label>
        <input
          id="title"
          type="text"
          className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
          {...register("title")}
        />
        {errors.title ? (
          <p role="alert" className="text-xs text-red-600">
            {errors.title.message}
          </p>
        ) : null}
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="priority" className="text-sm font-medium">
          Priority
        </label>
        <select
          id="priority"
          className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
          {...register("priority")}
        >
          {TASK_PRIORITIES.map((priority) => (
            <option key={priority} value={priority}>
              {priority}
            </option>
          ))}
        </select>
      </div>
      <Button type="submit" disabled={isSubmitting} className="self-start">
        Add task
      </Button>
    </form>
  );
}
