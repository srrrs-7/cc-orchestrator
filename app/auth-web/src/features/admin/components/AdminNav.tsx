import { Link } from "@tanstack/react-router";

type AdminNavProps = {
  readonly current: "home" | "users" | "clients" | "settings";
};

const NAV_ITEMS = [
  { id: "home" as const, label: "Overview", to: "/" as const },
  { id: "users" as const, label: "Users", to: "/users" as const },
  { id: "clients" as const, label: "OAuth clients", to: "/clients" as const },
  { id: "settings" as const, label: "Settings", to: "/settings" as const },
];

export function AdminNav({ current }: AdminNavProps) {
  return (
    <nav aria-label="Admin sections" className="flex flex-wrap gap-2">
      {NAV_ITEMS.map((item) => {
        const isActive = item.id === current;
        return (
          <Link
            key={item.id}
            to={item.to}
            className={`rounded-md px-3 py-2 text-sm font-medium transition-colors motion-reduce:transition-none pointer-coarse:min-h-11 pointer-coarse:inline-flex pointer-coarse:items-center focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 ${
              isActive ? "bg-accent text-white" : "bg-surface-muted text-gray-700 hover:bg-gray-200"
            }`}
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
