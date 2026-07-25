import { Link, Outlet } from "@tanstack/react-router";
import { LoginButton } from "../features/auth/components/LoginButton";
import { UserMenu } from "../features/auth/components/UserMenu";
import { useAuth } from "../features/auth/hooks/AuthProvider";
import { DEFAULT_LIMIT } from "../features/tasks/domain/pagination";

/** Shared layout: header + content outlet. Used as the root route's component. */
export function App() {
  const { isAuthenticated } = useAuth();

  return (
    <div className="min-h-screen bg-surface-muted text-gray-900">
      <header className="sticky top-0 z-10 border-b border-border-subtle bg-surface/95 shadow-sm backdrop-blur-sm">
        <div className="mx-auto flex max-w-3xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
          <Link
            to="/"
            search={{ status: "all", limit: DEFAULT_LIMIT, offset: 0 }}
            className="min-w-0 focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            <span className="block truncate text-lg font-semibold text-gray-900">Task Manager</span>
            <span className="block truncate text-xs text-gray-500">
              Organize and track your work
            </span>
          </Link>
          <div className="shrink-0">{isAuthenticated ? <UserMenu /> : <LoginButton />}</div>
        </div>
      </header>
      <main className="mx-auto max-w-3xl px-4 py-6 sm:px-6">
        <Outlet />
      </main>
    </div>
  );
}
