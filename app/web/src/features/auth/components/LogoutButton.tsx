import { Button } from "../../../shared/ui/Button";
import { useAuth } from "../hooks/AuthProvider";

/** Clears the session and navigates to /login. */
export function LogoutButton() {
  const { logout } = useAuth();

  return (
    <Button variant="secondary" onClick={logout}>
      Sign out
    </Button>
  );
}
