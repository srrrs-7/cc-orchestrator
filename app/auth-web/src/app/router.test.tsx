import { render, screen } from "@testing-library/react";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { describe, expect, it, beforeEach } from "vitest";
import { setStoredAdminApiKey } from "../features/admin/domain/credentials";
import { routeTree } from "./router.test-utils";

describe("router", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setStoredAdminApiKey("test-admin-key");
  });

  it("renders the overview page", async () => {
    const history = createMemoryHistory({ initialEntries: ["/"] });
    const router = createRouter({ routeTree, history });

    render(<RouterProvider router={router} />);

    expect(await screen.findByText("Authorization server provisioning")).toBeInTheDocument();
  });
});
