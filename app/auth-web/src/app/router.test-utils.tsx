import { QueryClientProvider } from "@tanstack/react-query";
import { createRootRoute, createRoute } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { AdminKeyGate } from "../features/admin/components/AdminKeyForm";
import { ClientForm } from "../features/admin/components/ClientForm";
import { ClientList } from "../features/admin/components/ClientList";
import { UserForm } from "../features/admin/components/UserForm";
import { UserList } from "../features/admin/components/UserList";
import { AdminAuthProvider } from "../features/admin/hooks/AdminAuthProvider";
import { Card, CardHeader } from "../shared/ui/Card";
import { queryClient } from "../lib/queryClient";
import { App } from "./App";

function TestProviders({ children }: { readonly children: ReactNode }) {
  return (
    <AdminAuthProvider>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </AdminAuthProvider>
  );
}

function OverviewPage() {
  return (
    <AdminKeyGate>
      <div className="flex flex-col gap-6">
        <Card className="p-4 sm:p-5">
          <CardHeader
            title="Authorization server provisioning"
            description="Use this console to register resource owners and OAuth clients before applications connect to the auth server."
          />
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

const rootRoute = createRootRoute({
  component: () => (
    <TestProviders>
      <App />
    </TestProviders>
  ),
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: OverviewPage,
});

export const routeTree = rootRoute.addChildren([indexRoute]);
