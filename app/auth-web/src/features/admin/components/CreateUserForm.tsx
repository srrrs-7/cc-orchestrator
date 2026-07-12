import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Alert } from "../../../shared/ui/Alert";
import { Button } from "../../../shared/ui/Button";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { Input } from "../../../shared/ui/Input";
import { createUserRequestSchema, type CreateUserRequest } from "../api/schema";
import { useCreateUser } from "../hooks/useAdminMutations";

export function CreateUserForm() {
  const createUser = useCreateUser();
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<CreateUserRequest>({
    resolver: zodResolver(createUserRequestSchema),
    defaultValues: {
      user_id: "",
      username: "",
      password: "",
      name: "",
      email: "",
    },
  });

  const onSubmit = handleSubmit((values) => {
    createUser.mutate(values, {
      onSuccess: () => reset(),
    });
  });

  return (
    <Card className="p-4 sm:p-5">
      <CardHeader
        title="Create user"
        description="Register a resource owner who can sign in and grant consent during authorization."
      />
      {createUser.isSuccess ? (
        <Alert variant="success" className="mb-4">
          User <strong>{createUser.data.username}</strong> ({createUser.data.user_id}) was created.
        </Alert>
      ) : null}
      {createUser.isError ? <Alert className="mb-4">{createUser.error.message}</Alert> : null}
      <form onSubmit={onSubmit} className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-1">
          <label htmlFor="user_id" className="text-sm font-medium text-gray-700">
            User ID (sub)
          </label>
          <Input
            id="user_id"
            placeholder="user-001"
            aria-invalid={errors.user_id !== undefined}
            {...register("user_id")}
          />
          {errors.user_id ? (
            <p role="alert" className="text-xs text-red-600">
              {errors.user_id.message}
            </p>
          ) : null}
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="username" className="text-sm font-medium text-gray-700">
            Username
          </label>
          <Input
            id="username"
            autoComplete="username"
            placeholder="jane.doe"
            aria-invalid={errors.username !== undefined}
            {...register("username")}
          />
          {errors.username ? (
            <p role="alert" className="text-xs text-red-600">
              {errors.username.message}
            </p>
          ) : null}
        </div>
        <div className="flex flex-col gap-1 sm:col-span-2">
          <label htmlFor="password" className="text-sm font-medium text-gray-700">
            Password
          </label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            aria-invalid={errors.password !== undefined}
            {...register("password")}
          />
          {errors.password ? (
            <p role="alert" className="text-xs text-red-600">
              {errors.password.message}
            </p>
          ) : null}
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="name" className="text-sm font-medium text-gray-700">
            Display name
          </label>
          <Input
            id="name"
            placeholder="Jane Doe"
            aria-invalid={errors.name !== undefined}
            {...register("name")}
          />
          {errors.name ? (
            <p role="alert" className="text-xs text-red-600">
              {errors.name.message}
            </p>
          ) : null}
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="email" className="text-sm font-medium text-gray-700">
            Email
          </label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            placeholder="jane@example.com"
            aria-invalid={errors.email !== undefined}
            {...register("email")}
          />
          {errors.email ? (
            <p role="alert" className="text-xs text-red-600">
              {errors.email.message}
            </p>
          ) : null}
        </div>
        <div className="sm:col-span-2">
          <Button type="submit" disabled={isSubmitting || createUser.isPending}>
            {createUser.isPending ? "Creating…" : "Create user"}
          </Button>
        </div>
      </form>
    </Card>
  );
}
