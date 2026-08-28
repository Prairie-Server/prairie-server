import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PlayerMoreMenu } from "./PlayerMoreMenu";

describe("PlayerMoreMenu", () => {
  it("exposes secondary player actions on mobile", () => {
    const onTogglePlaybackInfo = vi.fn();
    const onToggleMarkerEdit = vi.fn();
    const onTogglePiP = vi.fn();

    Object.defineProperty(document, "pictureInPictureEnabled", {
      configurable: true,
      value: true,
    });

    render(
      <PlayerMoreMenu
        markerEditAvailable
        onToggleMarkerEdit={onToggleMarkerEdit}
        onTogglePlaybackInfo={onTogglePlaybackInfo}
        onTogglePiP={onTogglePiP}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "More" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit markers" }));
    expect(onToggleMarkerEdit).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "More" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Stats for nerds" }));
    expect(onTogglePlaybackInfo).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "More" }));
    fireEvent.click(
      screen.getByRole("menuitem", { name: "Picture in Picture" }),
    );
    expect(onTogglePiP).toHaveBeenCalledOnce();
  });

  it("portals the menu to document.body", () => {
    render(<PlayerMoreMenu onTogglePlaybackInfo={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "More" }));
    const menu = screen.getByRole("menu");
    expect(menu.parentElement).toBe(document.body);
  });

  it("closes on outside pointerdown", () => {
    render(<PlayerMoreMenu onTogglePlaybackInfo={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "More" }));
    expect(screen.getByRole("menu")).toBeInTheDocument();

    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
