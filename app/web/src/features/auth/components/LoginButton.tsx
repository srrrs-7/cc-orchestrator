import { Button } from "../../../shared/ui/Button";
import { useAuth } from "../hooks/AuthProvider";

/** Triggers the OIDC Authorization Code + PKCE login redirect. */
export function LoginButton() {
  const { login } = useAuth();

  function handleClick() {
    void login();
  }

  return <Button onClick={handleClick}>Sign in</Button>;
}
