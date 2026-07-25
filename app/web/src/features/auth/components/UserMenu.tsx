import { useAuth } from "../hooks/AuthProvider";
import { LogoutButton } from "./LogoutButton";

/**
 * Shows the authenticated user's display name and a sign-out button.
 * Renders nothing when there is no authenticated user.
 */
export function UserMenu() {
  const { user } = useAuth();
  if (!user) return null;

  return (
    <div className="flex min-w-0 items-center gap-3">
      <span className="hidden min-w-0 truncate text-sm font-medium text-gray-700 sm:block sm:max-w-[12rem]">
        {user.displayName}
      </span>
      <LogoutButton />
    </div>
  );
}
