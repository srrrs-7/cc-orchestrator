import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AuthProvider } from "../hooks/AuthProvider";
import { LoginButton } from "./LoginButton";

function renderLoginButton() {
  return render(
    <AuthProvider>
      <LoginButton />
    </AuthProvider>,
  );
}

describe("LoginButton", () => {
  it("renders a 'Sign in' button", () => {
    renderLoginButton();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
  });

  it("the button is enabled by default", () => {
    renderLoginButton();
    expect(screen.getByRole("button", { name: "Sign in" })).not.toBeDisabled();
  });
});
