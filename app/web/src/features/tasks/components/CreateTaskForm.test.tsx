import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderWithQueryClient } from "../../../test/renderWithQueryClient";
import { CreateTaskForm } from "./CreateTaskForm";

describe("CreateTaskForm", () => {
  it("shows a validation error and does not submit when the title is empty (abnormal)", async () => {
    const user = userEvent.setup();
    renderWithQueryClient(<CreateTaskForm />);

    await user.click(screen.getByRole("button", { name: "Add task" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Title is required");
    // Validation failed client-side; the title input keeps focus/value
    // instead of being reset as it would be on a successful submit.
    expect(screen.getByLabelText("Title")).toHaveValue("");
  });

  it("submits successfully and resets the form when the input is valid (normal)", async () => {
    const user = userEvent.setup();
    renderWithQueryClient(<CreateTaskForm />);

    const titleInput = screen.getByLabelText("Title");
    await user.type(titleInput, "Write more tests");
    await user.selectOptions(screen.getByLabelText("Priority"), "high");
    await user.click(screen.getByRole("button", { name: "Add task" }));

    await waitFor(() => {
      expect(titleInput).toHaveValue("");
    });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
