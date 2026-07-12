import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { Alert } from "../../../shared/ui/Alert";
import { Button } from "../../../shared/ui/Button";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { Input } from "../../../shared/ui/Input";
import { Textarea } from "../../../shared/ui/Textarea";
import {
  createClientFormSchema,
  type CreateClientFormValues,
  toCreateClientRequest,
} from "../api/schema";
import {
  DEFAULT_GRANT_TYPES,
  DEFAULT_OAUTH_SCOPES,
  DEFAULT_RESPONSE_TYPES,
} from "../domain/scopes";
import { useCreateClient } from "../hooks/useAdminMutations";
import { ScopeSelector } from "./ScopeSelector";

const DEFAULT_REDIRECT_URIS = "http://localhost:8080/callback";

export function CreateClientForm() {
  const createClient = useCreateClient();
  const {
    register,
    control,
    handleSubmit,
    reset,
    watch,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<CreateClientFormValues>({
    resolver: zodResolver(createClientFormSchema),
    defaultValues: {
      client_id: "",
      redirect_uris_text: DEFAULT_REDIRECT_URIS,
      allowed_scopes: [...DEFAULT_OAUTH_SCOPES],
      response_types: [...DEFAULT_RESPONSE_TYPES],
      grant_types: [...DEFAULT_GRANT_TYPES],
      client_secret: "",
    },
  });

  const selectedScopes = watch("allowed_scopes");

  const onToggleScope = (scopeId: string, checked: boolean) => {
    const current = watch("allowed_scopes");
    if (checked) {
      setValue("allowed_scopes", [...new Set([...current, scopeId])], { shouldValidate: true });
      return;
    }
    if (scopeId === "openid") {
      return;
    }
    setValue(
      "allowed_scopes",
      current.filter((scope) => scope !== scopeId),
      { shouldValidate: true },
    );
  };

  const onSubmit = handleSubmit((values) => {
    createClient.mutate(toCreateClientRequest(values), {
      onSuccess: () => {
        reset({
          client_id: "",
          redirect_uris_text: DEFAULT_REDIRECT_URIS,
          allowed_scopes: [...DEFAULT_OAUTH_SCOPES],
          response_types: [...DEFAULT_RESPONSE_TYPES],
          grant_types: [...DEFAULT_GRANT_TYPES],
          client_secret: "",
        });
      },
    });
  });

  return (
    <Card className="p-4 sm:p-5">
      <CardHeader
        title="Create OAuth client"
        description="Define redirect URIs and the scopes this client may request during authorization."
      />
      {createClient.isSuccess ? (
        <Alert variant="success" className="mb-4">
          Client <strong>{createClient.data.client_id}</strong> was created
          {createClient.data.is_confidential ? " as a confidential client" : " as a public client"}.
        </Alert>
      ) : null}
      {createClient.isError ? <Alert className="mb-4">{createClient.error.message}</Alert> : null}
      <form onSubmit={onSubmit} className="flex flex-col gap-5">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="flex flex-col gap-1 sm:col-span-2">
            <label htmlFor="client_id" className="text-sm font-medium text-gray-700">
              Client ID
            </label>
            <Input
              id="client_id"
              placeholder="my-spa-client"
              aria-invalid={errors.client_id !== undefined}
              {...register("client_id")}
            />
            {errors.client_id ? (
              <p role="alert" className="text-xs text-red-600">
                {errors.client_id.message}
              </p>
            ) : null}
          </div>
          <div className="flex flex-col gap-1 sm:col-span-2">
            <label htmlFor="redirect_uris_text" className="text-sm font-medium text-gray-700">
              Redirect URIs
            </label>
            <Textarea
              id="redirect_uris_text"
              rows={3}
              placeholder="One URI per line"
              aria-invalid={errors.redirect_uris_text !== undefined}
              {...register("redirect_uris_text")}
            />
            {errors.redirect_uris_text ? (
              <p role="alert" className="text-xs text-red-600">
                {errors.redirect_uris_text.message}
              </p>
            ) : (
              <p className="text-xs text-gray-500">Enter one absolute URI per line.</p>
            )}
          </div>
          <div className="flex flex-col gap-1 sm:col-span-2">
            <label htmlFor="client_secret" className="text-sm font-medium text-gray-700">
              Client secret (optional)
            </label>
            <Input
              id="client_secret"
              type="password"
              autoComplete="new-password"
              placeholder="Leave empty for a public client"
              {...register("client_secret")}
            />
            <p className="text-xs text-gray-500">
              When provided, the client is registered as confidential (client_secret_post).
            </p>
          </div>
        </div>

        <Controller
          name="allowed_scopes"
          control={control}
          render={() => (
            <ScopeSelector
              selectedScopes={selectedScopes}
              onToggle={onToggleScope}
              errorMessage={errors.allowed_scopes?.message}
            />
          )}
        />

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-gray-700">Grant types</span>
            <p className="rounded-md border border-border-subtle bg-surface-muted/60 px-3 py-2 text-sm text-gray-700">
              {DEFAULT_GRANT_TYPES.join(", ")}
            </p>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-gray-700">Response types</span>
            <p className="rounded-md border border-border-subtle bg-surface-muted/60 px-3 py-2 text-sm text-gray-700">
              {DEFAULT_RESPONSE_TYPES.join(", ")}
            </p>
          </div>
        </div>

        <Button type="submit" disabled={isSubmitting || createClient.isPending}>
          {createClient.isPending ? "Creating…" : "Create client"}
        </Button>
      </form>
    </Card>
  );
}
