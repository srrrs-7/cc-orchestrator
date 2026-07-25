import { zodResolver } from "@hookform/resolvers/zod";
import { Link, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { Alert } from "../../../shared/ui/Alert";
import { Button } from "../../../shared/ui/Button";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { Input } from "../../../shared/ui/Input";
import {
  createUserRequestSchema,
  type CreateUserRequest,
  updateUserRequestSchema,
  type UpdateUserRequest,
} from "../api/schema";
import { useCreateUser, useUpdateUser } from "../hooks/useAdminMutations";
import { useUserQuery } from "../hooks/useAdminQueries";

type UserFormProps = {
  readonly mode: "create" | "edit";
  readonly userId?: string;
};

export function UserForm({ mode, userId }: UserFormProps) {
  const navigate = useNavigate();
  const createUser = useCreateUser();
  const isEdit = mode === "edit";
  const resolvedUserId = userId ?? "";
  const updateUser = useUpdateUser(resolvedUserId);
  const { data: existingUser, isLoading, isError, error } = useUserQuery(resolvedUserId, isEdit);

  const createForm = useForm<CreateUserRequest>({
    resolver: zodResolver(createUserRequestSchema),
    defaultValues: {
      user_id: "",
      username: "",
      password: "",
      name: "",
      email: "",
    },
  });

  const editForm = useForm<UpdateUserRequest>({
    resolver: zodResolver(updateUserRequestSchema),
    defaultValues: {
      username: "",
      password: "",
      name: "",
      email: "",
    },
  });

  useEffect(() => {
    if (!isEdit || existingUser === undefined) {
      return;
    }
    editForm.reset({
      username: existingUser.username,
      password: "",
      name: existingUser.name,
      email: existingUser.email,
    });
  }, [isEdit, existingUser, editForm]);

  if (isEdit && isLoading) {
    return <p className="text-sm text-gray-500">Loading user…</p>;
  }

  if (isEdit && isError) {
    return (
      <div className="flex flex-col gap-4">
        <Alert>{error.message}</Alert>
        <Link
          to="/users"
          className="text-sm font-medium text-accent hover:text-accent-hover pointer-coarse:min-h-11 pointer-coarse:inline-flex pointer-coarse:items-center"
        >
          ← Back to users
        </Link>
      </div>
    );
  }

  if (isEdit) {
    const onSubmit = editForm.handleSubmit((values) => {
      updateUser.mutate(values, {
        onSuccess: () => {
          void navigate({ to: "/users" });
        },
      });
    });

    return (
      <Card className="p-4 sm:p-5">
        <CardHeader
          title="Edit user"
          description={`Update profile details for ${resolvedUserId}. Leave password blank to keep the current value.`}
        />
        {updateUser.isError ? <Alert className="mb-4">{updateUser.error.message}</Alert> : null}
        <form onSubmit={onSubmit} className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="flex flex-col gap-1 sm:col-span-2">
            <span className="text-sm font-medium text-gray-700">User ID (sub)</span>
            <p className="rounded-md border border-border-subtle bg-surface-muted/60 px-3 py-2 font-mono text-sm text-gray-800">
              {resolvedUserId}
            </p>
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor="edit-username" className="text-sm font-medium text-gray-700">
              Username
            </label>
            <Input id="edit-username" {...editForm.register("username")} />
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor="edit-password" className="text-sm font-medium text-gray-700">
              New password (optional)
            </label>
            <Input
              id="edit-password"
              type="password"
              autoComplete="new-password"
              placeholder="Leave blank to keep current password"
              {...editForm.register("password")}
            />
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor="edit-name" className="text-sm font-medium text-gray-700">
              Display name
            </label>
            <Input id="edit-name" {...editForm.register("name")} />
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor="edit-email" className="text-sm font-medium text-gray-700">
              Email
            </label>
            <Input id="edit-email" type="email" {...editForm.register("email")} />
          </div>
          <div className="flex flex-wrap gap-2 sm:col-span-2">
            <Button type="submit" disabled={updateUser.isPending}>
              {updateUser.isPending ? "Saving…" : "Save changes"}
            </Button>
            <Link to="/users">
              <Button type="button" variant="secondary">
                Cancel
              </Button>
            </Link>
          </div>
        </form>
      </Card>
    );
  }

  const onSubmit = createForm.handleSubmit((values) => {
    createUser.mutate(values, {
      onSuccess: () => createForm.reset(),
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
          <Input id="user_id" placeholder="user-001" {...createForm.register("user_id")} />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="username" className="text-sm font-medium text-gray-700">
            Username
          </label>
          <Input id="username" autoComplete="username" {...createForm.register("username")} />
        </div>
        <div className="flex flex-col gap-1 sm:col-span-2">
          <label htmlFor="password" className="text-sm font-medium text-gray-700">
            Password
          </label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            {...createForm.register("password")}
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="name" className="text-sm font-medium text-gray-700">
            Display name
          </label>
          <Input id="name" {...createForm.register("name")} />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="email" className="text-sm font-medium text-gray-700">
            Email
          </label>
          <Input id="email" type="email" autoComplete="email" {...createForm.register("email")} />
        </div>
        <div className="sm:col-span-2">
          <Button type="submit" disabled={createUser.isPending}>
            {createUser.isPending ? "Creating…" : "Create user"}
          </Button>
        </div>
      </form>
    </Card>
  );
}
