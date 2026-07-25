import { Link } from "@tanstack/react-router";
import { Alert } from "../../../shared/ui/Alert";
import { Badge } from "../../../shared/ui/Badge";
import { Button } from "../../../shared/ui/Button";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { formatScopeList, formatUriList } from "../domain/format";
import { useDeleteClient } from "../hooks/useAdminMutations";
import { useClientsQuery } from "../hooks/useAdminQueries";

export function ClientList() {
  const { data, isLoading, isError, error } = useClientsQuery();
  const deleteClient = useDeleteClient();

  const handleDelete = (clientId: string) => {
    const confirmed = window.confirm(
      `Delete OAuth client "${clientId}" and related consent/tokens? This cannot be undone.`,
    );
    if (!confirmed) {
      return;
    }
    deleteClient.mutate(clientId);
  };

  return (
    <Card className="p-4 sm:p-5">
      <CardHeader
        title="Registered OAuth clients"
        description="Applications and their allowed redirect URIs and authorization scopes."
      />
      {deleteClient.isError ? <Alert className="mb-4">{deleteClient.error.message}</Alert> : null}
      {isLoading ? <p className="text-sm text-gray-500">Loading clients…</p> : null}
      {isError ? <Alert className="mb-4">{error.message}</Alert> : null}
      {!isLoading && !isError && (data === undefined || data.length === 0) ? (
        <p className="text-sm text-gray-500">No OAuth clients registered yet.</p>
      ) : null}
      {data !== undefined && data.length > 0 ? (
        <div className="flex flex-col gap-4">
          {data.map((client) => (
            <article
              key={client.client_id}
              className="rounded-lg border border-border-subtle bg-surface-muted/50 p-4"
            >
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <h3 className="break-all font-mono text-sm font-semibold text-gray-900">
                    {client.client_id}
                  </h3>
                  <Badge>{client.is_confidential ? "Confidential" : "Public"}</Badge>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Link
                    to="/clients/$clientId/edit"
                    params={{ clientId: client.client_id }}
                    className="pointer-coarse:min-h-11 pointer-coarse:inline-flex pointer-coarse:items-center"
                  >
                    <Button type="button" variant="secondary">
                      Edit
                    </Button>
                  </Link>
                  <Button
                    type="button"
                    variant="secondary"
                    disabled={deleteClient.isPending}
                    onClick={() => handleDelete(client.client_id)}
                  >
                    Delete
                  </Button>
                </div>
              </div>
              <dl className="mt-3 grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
                <div className="min-w-0 sm:col-span-2">
                  <dt className="font-medium text-gray-700">Redirect URIs</dt>
                  <dd className="mt-1 whitespace-pre-wrap break-all font-mono text-xs text-gray-600">
                    {formatUriList(client.redirect_uris)}
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="font-medium text-gray-700">Allowed scopes</dt>
                  <dd className="mt-1 break-words text-gray-600">
                    {formatScopeList(client.allowed_scopes)}
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="font-medium text-gray-700">Grant types</dt>
                  <dd className="mt-1 break-words text-gray-600">
                    {formatScopeList(client.grant_types)}
                  </dd>
                </div>
              </dl>
            </article>
          ))}
        </div>
      ) : null}
    </Card>
  );
}
