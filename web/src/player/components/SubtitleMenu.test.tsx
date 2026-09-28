// @vitest-environment jsdom

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SubtitleMenu } from "./SubtitleMenu";
import type { PlayerConfig } from "../context/PlayerConfigContext";

const { playerV2Mock } = vi.hoisted(() => ({
  playerV2Mock: vi.fn(),
}));

vi.mock("../player-v2", () => ({
  playerV2: playerV2Mock,
}));

const AI_STATUS = "GET /api/v2/subtitles/ai/status";

function aiStatusCalls() {
  return playerV2Mock.mock.calls.filter(([, route]) => route === AI_STATUS);
}

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
    playerV2Mock.mockReset();
  });

  it("does not probe AI subtitle status until the menu opens", async () => {
    playerV2Mock.mockResolvedValue({
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

    expect(aiStatusCalls()).toHaveLength(0);

    await userEvent.click(screen.getByRole("button", { name: /enable captions/i }));

    await waitFor(() => {
      expect(playerV2Mock).toHaveBeenCalledWith(config, AI_STATUS, {});
    });
  });

  it("probes AI subtitle status only once per menu session", async () => {
    playerV2Mock.mockResolvedValue({
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
    await waitFor(() => expect(aiStatusCalls()).toHaveLength(1));

    await userEvent.click(trigger);
    await userEvent.click(trigger);

    expect(aiStatusCalls()).toHaveLength(1);
  });
});
