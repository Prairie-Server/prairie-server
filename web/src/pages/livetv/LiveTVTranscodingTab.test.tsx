import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { LiveTVTuner } from "@/api/types";
import { LiveTVTranscodingTab } from "./LiveTVTranscodingTab";

const settingsForm = {
  getValue: (key: string) =>
    ({
      "livetv.hw_accel": "auto",
      "livetv.hw_decode": "auto",
      "livetv.encoder_preset": "low_latency",
      "livetv.framerate_cap": "source",
      "livetv.max_resolution": "source",
      "livetv.play_method": "auto",
      "livetv.max_transcodes": "3",
    })[key] ?? "",
  setValue: vi.fn(),
  save: vi.fn(),
  discard: vi.fn(),
  dirtyCount: 0,
  isSaving: false,
  isLoading: false,
  restartRequired: false,
};

let tuners: LiveTVTuner[] = [];

vi.mock("@/hooks/useSettingsForm", () => ({
  useSettingsForm: () => settingsForm,
}));

vi.mock("@/hooks/queries/useLiveTV", () => ({
  useLiveTVTuners: () => ({ data: tuners, isLoading: false }),
}));

function tuner(overrides: Partial<LiveTVTuner> = {}): LiveTVTuner {
  return {
    id: "t1",
    type: "hdhomerun",
    device_id: "1234ABCD",
    discover_url: "",
    base_url: "",
    model: "HDHR5-4K",
    firmware: "",
    tuner_count: 4,
    status: "ready",
    channel_count: 12,
    last_error: "",
    ...overrides,
  };
}

describe("LiveTVTranscodingTab", () => {
  it("renders the pipeline controls", () => {
    tuners = [];
    render(<LiveTVTranscodingTab />);

    expect(screen.getByText("Hardware acceleration")).toBeInTheDocument();
    expect(screen.getByText("Hardware decoding")).toBeInTheDocument();
    expect(screen.getByText("Encoder preset")).toBeInTheDocument();
    expect(screen.getByText("Frame rate cap")).toBeInTheDocument();
    expect(screen.getByText("Play method")).toBeInTheDocument();
  });

  // Current tuners ignore ?transcode= silently, so the UI must say the tuner
  // cannot do it rather than offering a profile that would be dropped.
  it("marks a tuner without device transcoding as unsupported", () => {
    tuners = [tuner()];
    render(<LiveTVTranscodingTab />);

    expect(screen.getByText("Not supported by this tuner")).toBeInTheDocument();
    expect(screen.getByText(/No tuner reports device-side transcoding/)).toBeInTheDocument();
  });

  it("lists the profiles a capable tuner advertises", () => {
    tuners = [tuner({ model: "HDHR EXTEND", transcode_codecs: ["heavy", "mobile"] })];
    render(<LiveTVTranscodingTab />);

    expect(screen.getByText("heavy")).toBeInTheDocument();
    expect(screen.getByText("mobile")).toBeInTheDocument();
    expect(screen.queryByText("Not supported by this tuner")).not.toBeInTheDocument();
  });
});
