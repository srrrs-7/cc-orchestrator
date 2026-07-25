import { createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { z } from "zod";
import { App } from "./App";
import { AdminKeyGate } from "../features/admin/components/AdminKeyForm";
import { ClientForm } from "../features/admin/components/ClientForm";
import { ClientList } from "../features/admin/components/ClientList";
import { UserForm } from "../features/admin/components/UserForm";
import { UserList } from "../features/admin/components/UserList";
import { AdminKeyForm } from "../features/admin/components/AdminKeyForm";
import { Card, CardHeader } from "../shared/ui/Card";
import { Alert } from "../shared/ui/Alert";
import { useAdminAuth } from "../features/admin/hooks/AdminAuthProvider";

const userIdParamsSchema = z.object({
  userId: z.string().min(1),
});

const clientIdParamsSchema = z.object({
  clientId: z.string().min(1),
});

function OverviewPage() {
  return (
    <AdminKeyGate>
      <div className="flex flex-col gap-6">
        <Card className="p-4 sm:p-5">
          <CardHeader
            title="Authorization server provisioning"
            description="Use this console to register resource owners and OAuth clients before applications connect to the auth server."
          />
          <ul className="list-disc space-y-2 pl-5 text-sm text-gray-700">
            <li>
              <strong>Users</strong> — create, edit, and delete accounts that can sign in and
              approve consent.
            </li>
            <li>
              <strong>OAuth clients</strong> — register redirect URIs and define which scopes each
              application may request.
            </li>
            <li>
              <strong>Settings</strong> — update the admin API key stored for this browser session.
            </li>
          </ul>
        </Card>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <div className="flex flex-col gap-6">
            <UserList />
            <UserForm mode="create" />
          </div>
          <div className="flex flex-col gap-6">
            <ClientList />
            <ClientForm mode="create" />
          </div>
        </div>
      </div>
    </AdminKeyGate>
  );
}

function UsersPage() {
  return (
    <AdminKeyGate>
      <div className="flex flex-col gap-6">
        <UserList />
        <UserForm mode="create" />
      </div>
    </AdminKeyGate>
  );
}

function ClientsPage() {
  return (
    <AdminKeyGate>
      <div className="flex flex-col gap-6">
        <ClientList />
        <ClientForm mode="create" />
      </div>
    </AdminKeyGate>
  );
}

function SettingsPage() {
  const { isConfigured, apiKey } = useAdminAuth();

  return (
    <div className="flex flex-col gap-4">
      {isConfigured ? (
        <Alert variant="info">
          An admin API key is configured for this session
          {apiKey !== null ? ` (ending in …${apiKey.slice(-4)})` : ""}.
        </Alert>
      ) : (
        <Alert variant="info">No admin API key is configured yet.</Alert>
      )}
      <AdminKeyForm />
    </div>
  );
}

const rootRoute = createRootRoute({
  component: App,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: OverviewPage,
});

const usersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/users",
  component: UsersPage,
});

const editUserRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/users/$userId/edit",
  params: {
    parse: (rawParams) => userIdParamsSchema.parse(rawParams),
  },
  component: function EditUserRoute() {
    const { userId } = editUserRoute.useParams();
    return (
      <AdminKeyGate>
        <UserForm mode="edit" userId={userId} />
      </AdminKeyGate>
    );
  },
});

const clientsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/clients",
  component: ClientsPage,
});

const editClientRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/clients/$clientId/edit",
  params: {
    parse: (rawParams) => clientIdParamsSchema.parse(rawParams),
  },
  component: function EditClientRoute() {
    const { clientId } = editClientRoute.useParams();
    return (
      <AdminKeyGate>
        <ClientForm mode="edit" clientId={clientId} />
      </AdminKeyGate>
    );
  },
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  usersRoute,
  editUserRoute,
  clientsRoute,
  editClientRoute,
  settingsRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
