import { Alert } from "../../../shared/ui/Alert";
import { Badge } from "../../../shared/ui/Badge";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { formatScopeList, formatUriList } from "../domain/format";
import { useClientsQuery } from "../hooks/useAdminQueries";

export function ClientList() {
  const { data, isLoading, isError, error } = useClientsQuery();

  return (
    <Card className="p-4 sm:p-5">
      <CardHeader
        title="Registered OAuth clients"
        description="Applications and their allowed redirect URIs and authorization scopes."
      />
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
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="break-all font-mono text-sm font-semibold text-gray-900">
                  {client.client_id}
                </h3>
                <Badge>{client.is_confidential ? "Confidential" : "Public"}</Badge>
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
