import { QueryClientProvider } from "@tanstack/react-query";
import { createRootRoute, createRoute } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { AdminKeyForm, AdminKeyGate } from "../features/admin/components/AdminKeyForm";
import { CreateClientForm } from "../features/admin/components/CreateClientForm";
import { CreateUserForm } from "../features/admin/components/CreateUserForm";
import { AdminAuthProvider, useAdminAuth } from "../features/admin/hooks/AdminAuthProvider";
import { queryClient } from "../lib/queryClient";
import { Alert } from "../shared/ui/Alert";
import { Card, CardHeader } from "../shared/ui/Card";
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
      <Card className="p-4">
        <CardHeader title="Authorization server provisioning" />
      </Card>
      <CreateUserForm />
      <CreateClientForm />
    </AdminKeyGate>
  );
}

function SettingsPage() {
  const { isConfigured } = useAdminAuth();
  return (
    <div>
      {isConfigured ? (
        <Alert variant="info">Configured</Alert>
      ) : (
        <Alert variant="info">Missing</Alert>
      )}
      <AdminKeyForm />
    </div>
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

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
});

export const routeTree = rootRoute.addChildren([indexRoute, settingsRoute]);
