import { zodResolver } from "@hookform/resolvers/zod";
import { Link, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
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
  toUpdateClientRequest,
  updateClientRequestSchema,
  type UpdateClientFormValues,
} from "../api/schema";
import {
  DEFAULT_GRANT_TYPES,
  DEFAULT_OAUTH_SCOPES,
  DEFAULT_RESPONSE_TYPES,
} from "../domain/scopes";
import { formatRedirectUris } from "../domain/redirectUris";
import { useCreateClient, useUpdateClient } from "../hooks/useAdminMutations";
import { useClientQuery } from "../hooks/useAdminQueries";
import { ScopeSelector } from "./ScopeSelector";

const DEFAULT_REDIRECT_URIS = "http://localhost:8080/callback";

type ClientFormProps = {
  readonly mode: "create" | "edit";
  readonly clientId?: string;
};

export function ClientForm({ mode, clientId }: ClientFormProps) {
  const navigate = useNavigate();
  const createClient = useCreateClient();
  const isEdit = mode === "edit";
  const resolvedClientId = clientId ?? "";
  const updateClient = useUpdateClient(resolvedClientId);
  const {
    data: existingClient,
    isLoading,
    isError,
    error,
  } = useClientQuery(resolvedClientId, isEdit);

  const createForm = useForm<CreateClientFormValues>({
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

  const editForm = useForm<UpdateClientFormValues>({
    resolver: zodResolver(updateClientRequestSchema),
    defaultValues: {
      redirect_uris_text: DEFAULT_REDIRECT_URIS,
      allowed_scopes: [...DEFAULT_OAUTH_SCOPES],
      response_types: [...DEFAULT_RESPONSE_TYPES],
      grant_types: [...DEFAULT_GRANT_TYPES],
      client_secret: "",
    },
  });

  useEffect(() => {
    if (!isEdit || existingClient === undefined) {
      return;
    }
    editForm.reset({
      redirect_uris_text: formatRedirectUris(existingClient.redirect_uris),
      allowed_scopes: existingClient.allowed_scopes,
      response_types: existingClient.response_types,
      grant_types: existingClient.grant_types,
      client_secret: "",
    });
  }, [isEdit, existingClient, editForm]);

  const toggleScopeSelection = (current: string[], scopeId: string, checked: boolean): string[] => {
    if (checked) {
      return [...new Set([...current, scopeId])];
    }
    if (scopeId === "openid") {
      return current;
    }
    return current.filter((scope) => scope !== scopeId);
  };

  if (isEdit && isLoading) {
    return <p className="text-sm text-gray-500">Loading client…</p>;
  }

  if (isEdit && isError) {
    return (
      <div className="flex flex-col gap-4">
        <Alert>{error.message}</Alert>
        <Link
          to="/clients"
          className="text-sm font-medium text-accent hover:text-accent-hover pointer-coarse:min-h-11 pointer-coarse:inline-flex pointer-coarse:items-center"
        >
          ← Back to OAuth clients
        </Link>
      </div>
    );
  }

  if (isEdit) {
    const selectedScopes = editForm.watch("allowed_scopes");
    const onSubmit = editForm.handleSubmit((values) => {
      updateClient.mutate(toUpdateClientRequest(values), {
        onSuccess: () => {
          void navigate({ to: "/clients" });
        },
      });
    });

    return (
      <Card className="p-4 sm:p-5">
        <CardHeader
          title="Edit OAuth client"
          description={`Update redirect URIs and scopes for ${resolvedClientId}. Leave client secret blank to keep the current confidential-client hash.`}
        />
        {updateClient.isError ? <Alert className="mb-4">{updateClient.error.message}</Alert> : null}
        <form onSubmit={onSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium text-gray-700">Client ID</span>
            <p className="rounded-md border border-border-subtle bg-surface-muted/60 px-3 py-2 font-mono text-sm text-gray-800">
              {resolvedClientId}
            </p>
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor="edit-redirect_uris_text" className="text-sm font-medium text-gray-700">
              Redirect URIs
            </label>
            <Textarea
              id="edit-redirect_uris_text"
              rows={3}
              {...editForm.register("redirect_uris_text")}
            />
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor="edit-client_secret" className="text-sm font-medium text-gray-700">
              New client secret (optional)
            </label>
            <Input
              id="edit-client_secret"
              type="password"
              placeholder="Leave blank to keep the current secret"
              {...editForm.register("client_secret")}
            />
          </div>
          <Controller
            name="allowed_scopes"
            control={editForm.control}
            render={() => (
              <ScopeSelector
                selectedScopes={selectedScopes}
                onToggle={(scopeId, checked) => {
                  const current = editForm.getValues("allowed_scopes");
                  editForm.setValue(
                    "allowed_scopes",
                    toggleScopeSelection(current, scopeId, checked),
                    { shouldValidate: true },
                  );
                }}
              />
            )}
          />
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={updateClient.isPending}>
              {updateClient.isPending ? "Saving…" : "Save changes"}
            </Button>
            <Link to="/clients">
              <Button type="button" variant="secondary">
                Cancel
              </Button>
            </Link>
          </div>
        </form>
      </Card>
    );
  }

  const selectedScopes = createForm.watch("allowed_scopes");
  const onSubmit = createForm.handleSubmit((values) => {
    createClient.mutate(toCreateClientRequest(values), {
      onSuccess: () => {
        createForm.reset({
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
        <div className="flex flex-col gap-1">
          <label htmlFor="client_id" className="text-sm font-medium text-gray-700">
            Client ID
          </label>
          <Input id="client_id" placeholder="my-spa-client" {...createForm.register("client_id")} />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="redirect_uris_text" className="text-sm font-medium text-gray-700">
            Redirect URIs
          </label>
          <Textarea
            id="redirect_uris_text"
            rows={3}
            {...createForm.register("redirect_uris_text")}
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="client_secret" className="text-sm font-medium text-gray-700">
            Client secret (optional)
          </label>
          <Input id="client_secret" type="password" {...createForm.register("client_secret")} />
        </div>
        <Controller
          name="allowed_scopes"
          control={createForm.control}
          render={() => (
            <ScopeSelector
              selectedScopes={selectedScopes}
              onToggle={(scopeId, checked) => {
                const current = createForm.getValues("allowed_scopes");
                createForm.setValue(
                  "allowed_scopes",
                  toggleScopeSelection(current, scopeId, checked),
                  { shouldValidate: true },
                );
              }}
            />
          )}
        />
        <Button type="submit" disabled={createClient.isPending}>
          {createClient.isPending ? "Creating…" : "Create client"}
        </Button>
      </form>
    </Card>
  );
}
