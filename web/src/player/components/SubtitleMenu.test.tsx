// @vitest-environment jsdom

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SubtitleMenu } from "./SubtitleMenu";
import type { PlayerConfig } from "../context/PlayerConfigContext";

const { playerFetchMock } = vi.hoisted(() => ({
  playerFetchMock: vi.fn(),
}));

vi.mock("../player-fetch", () => ({
  playerFetch: playerFetchMock,
}));

vi.mock("./SubtitleSearchModal", () => ({
  SubtitleSearchModal: () => null,
}));

vi.mock("./SubtitleTranslateModal", () => ({
  SubtitleTranslateModal: () => null,
}));

vi.mock("./SubtitleAppearancePanel", () => ({
  SubtitleAppearancePanel: () => null,
}));

const config: PlayerConfig = {
  apiBaseUrl: "/api/v1",
  getAccessToken: () => null,
  getProfileId: () => null,
  getDeviceId: () => "test-device",
};

describe("SubtitleMenu", () => {
  afterEach(() => {
    playerFetchMock.mockReset();
  });

  it("does not probe AI subtitle status until the menu opens", async () => {
    playerFetchMock.mockResolvedValue({
      enabled: false,
      transcribe_enabled: false,
    });

    render(
      <SubtitleMenu
        tracks={[]}
        activeIndex={null}
        onSelect={vi.fn()}
        delayMs={0}
        onDelayChange={vi.fn()}
        mediaFileId={318}
        playerConfig={config}
        audioTracks={[]}
      />,
    );

    expect(playerFetchMock).not.toHaveBeenCalled();

    await userEvent.click(
      screen.getByRole("button", { name: /enable captions/i }),
    );

    await waitFor(() => {
      expect(playerFetchMock).toHaveBeenCalledWith(
        config,
        "/subtitles/ai/status",
      );
    });
  });

  it("probes AI subtitle status only once per menu session", async () => {
    playerFetchMock.mockResolvedValue({
      enabled: false,
      transcribe_enabled: false,
    });

    render(
      <SubtitleMenu
        tracks={[]}
        activeIndex={null}
        onSelect={vi.fn()}
        delayMs={0}
        onDelayChange={vi.fn()}
        mediaFileId={318}
        playerConfig={config}
        audioTracks={[]}
      />,
    );

    const trigger = screen.getByRole("button", { name: /enable captions/i });
    await userEvent.click(trigger);
    await waitFor(() => expect(playerFetchMock).toHaveBeenCalledTimes(1));

    await userEvent.click(trigger);
    await userEvent.click(trigger);

    expect(playerFetchMock).toHaveBeenCalledTimes(1);
  });
});
