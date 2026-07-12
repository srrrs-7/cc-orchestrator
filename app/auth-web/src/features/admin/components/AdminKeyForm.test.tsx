import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { AdminAuthProvider } from "../hooks/AdminAuthProvider";
import { AdminKeyGate } from "./AdminKeyForm";

function renderGate() {
  return render(
    <AdminAuthProvider>
      <AdminKeyGate>
        <p>Protected content</p>
      </AdminKeyGate>
    </AdminAuthProvider>,
  );
}

describe("AdminKeyGate", () => {
  it("shows the API key form before content is available", () => {
    renderGate();
    expect(screen.getByRole("heading", { name: "Admin API key" })).toBeInTheDocument();
    expect(screen.queryByText("Protected content")).not.toBeInTheDocument();
  });

  it("reveals content after saving a key", async () => {
    const user = userEvent.setup();
    renderGate();

    await user.type(screen.getByLabelText("API key"), "test-admin-key");
    await user.click(screen.getByRole("button", { name: "Save key" }));

    expect(await screen.findByText("Protected content")).toBeInTheDocument();
  });
});
