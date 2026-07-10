import { Link, Outlet } from "@tanstack/react-router";
import { DEFAULT_LIMIT } from "../features/tasks/domain/pagination";

/** Shared layout: header + content outlet. Used as the root route's component. */
export function App() {
  return (
    <div className="min-h-screen bg-gray-50 text-gray-900">
      <header className="border-b border-gray-200 bg-white">
        <div className="mx-auto max-w-3xl px-4 py-3 sm:px-6">
          <Link
            to="/"
            search={{ status: "all", limit: DEFAULT_LIMIT, offset: 0 }}
            className="text-lg font-semibold"
          >
            Task Manager
          </Link>
        </div>
      </header>
      <main className="mx-auto max-w-3xl px-4 py-6 sm:px-6">
        <Outlet />
      </main>
    </div>
  );
}
