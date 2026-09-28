import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import FilterChip from "./FilterChip";

describe("FilterChip", () => {
  it("renders field name and value", () => {
    render(
      <FilterChip chip={{ field: "genre", op: "contains", value: "Sci-Fi" }} onRemove={() => {}} />,
    );
    expect(screen.getByText(/genre/i)).toBeInTheDocument();
    expect(screen.getByText("Sci-Fi")).toBeInTheDocument();
  });

  it("calls onRemove when × clicked", async () => {
    const onRemove = vi.fn();
    render(<FilterChip chip={{ field: "year", op: "is", value: 1985 }} onRemove={onRemove} />);
    await userEvent.click(screen.getByRole("button", { name: /remove/i }));
    expect(onRemove).toHaveBeenCalled();
  });
});
