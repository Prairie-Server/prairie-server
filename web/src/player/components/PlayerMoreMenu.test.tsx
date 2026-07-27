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
    fireEvent.click(screen.getByRole("menuitem", { name: "Playback info" }));
    expect(onTogglePlaybackInfo).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "More" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Picture in Picture" }));
    expect(onTogglePiP).toHaveBeenCalledOnce();
  });
});
