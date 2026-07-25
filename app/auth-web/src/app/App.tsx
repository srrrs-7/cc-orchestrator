import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { AdminNav } from "../features/admin/components/AdminNav";
import { useAdminAuth } from "../features/admin/hooks/AdminAuthProvider";
import { Button } from "../shared/ui/Button";

type NavId = "home" | "users" | "clients" | "settings";

function navFromPathname(pathname: string): NavId {
  if (pathname.startsWith("/users")) return "users";
  if (pathname.startsWith("/clients")) return "clients";
  if (pathname.startsWith("/settings")) return "settings";
  return "home";
}

export function App() {
  const { isConfigured, clearApiKey } = useAdminAuth();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const nav = navFromPathname(pathname);

  return (
    <div className="min-h-screen bg-surface-muted text-gray-900">
      <header className="sticky top-0 z-10 border-b border-border-subtle bg-surface/95 shadow-sm backdrop-blur-sm">
        <div className="mx-auto flex max-w-4xl flex-col gap-4 px-4 py-4 sm:px-6">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <Link
              to="/"
              className="min-w-0 focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
            >
              <span className="block truncate text-lg font-semibold text-gray-900">Auth Admin</span>
              <span className="block truncate text-xs text-gray-500">
                Users, OAuth clients, and authorization scopes
              </span>
            </Link>
            {isConfigured ? (
              <Button variant="secondary" type="button" onClick={clearApiKey}>
                Clear API key
              </Button>
            ) : null}
          </div>
          <AdminNav current={nav} />
        </div>
      </header>
      <main className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
        <Outlet />
      </main>
    </div>
  );
}
