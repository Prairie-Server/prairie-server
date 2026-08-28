import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import AddFilterPopover from "./AddFilterPopover";

describe("AddFilterPopover", () => {
  it("submits chip on Add", async () => {
    const onAdd = vi.fn();
    render(<AddFilterPopover open onAdd={onAdd} onCancel={() => {}} />);
    await userEvent.selectOptions(screen.getByLabelText(/field/i), "genre");
    await userEvent.selectOptions(
      screen.getByLabelText(/operator/i),
      "contains",
    );
    await userEvent.type(screen.getByLabelText(/value/i), "Drama");
    await userEvent.click(screen.getByRole("button", { name: /add/i }));
    expect(onAdd).toHaveBeenCalledWith({
      field: "genre",
      op: "contains",
      value: "Drama",
    });
  });

  it("does not submit when field is blank", async () => {
    const onAdd = vi.fn();
    render(<AddFilterPopover open onAdd={onAdd} onCancel={() => {}} />);
    await userEvent.click(screen.getByRole("button", { name: /add/i }));
    expect(onAdd).not.toHaveBeenCalled();
  });
});
