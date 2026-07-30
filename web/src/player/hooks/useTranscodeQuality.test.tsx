import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTranscodeQuality } from "./useTranscodeQuality";
import { PlayerConfigProvider } from "../context/PlayerConfigContext";
import type { PlayerConfig } from "../context/PlayerConfigContext";
import type { PlayerFileVersion, TranscodeStartRequest } from "../types";

// The ladder now comes from the server, and its request would otherwise land on
// this file's fetch mock and shift every call count here. It is a separate unit
// with its own tests (useQualityLadder.test.tsx), so stub it to the production
// rungs and keep these assertions about transcode starts alone. Without this the
// counts are also order-dependent, because the real hook caches at module scope.
vi.mock("./useQualityLadder", () => ({
  useQualityLadder: () => [
    { id: "2160p", label: "4K", resolution: "2160p", height: 2160, bitrate_kbps: 20000 },
    {
      id: "1080p-high",
      label: "1080p High",
      resolution: "1080p",
      height: 1080,
      bitrate_kbps: 10000,
    },
    { id: "1080p", label: "1080p", resolution: "1080p", height: 1080, bitrate_kbps: 6000 },
    { id: "720p-high", label: "720p High", resolution: "720p", height: 720, bitrate_kbps: 4000 },
    { id: "720p", label: "720p", resolution: "720p", height: 720, bitrate_kbps: 2000 },
    { id: "480p", label: "480p", resolution: "480p", height: 480, bitrate_kbps: 1500 },
    { id: "420p", label: "420p", resolution: "420p", height: 420, bitrate_kbps: 720 },
  ],
}));

const config: PlayerConfig = {
  apiBaseUrl: "/api/v1",
  getAccessToken: () => null,
  getProfileId: () => null,
};

const version: PlayerFileVersion = {
  file_id: 42,
  resolution: "1080p",
  codec_video: "h264",
  codec_audio: "aac",
  hdr: false,
  container: "mkv",
  file_size: 1_000_000,
  duration: 7200,
  bitrate: 8000,
};

function wrapper({ children }: { children: ReactNode }) {
  return <PlayerConfigProvider config={config}>{children}</PlayerConfigProvider>;
}

function transcodeStartResponse(overrides: Record<string, unknown> = {}) {
  return {
    ok: true,
    status: 200,
    json: async () => ({
      session_id: "sess-1",
      status: "started",
      manifest_url: "/api/v1/playback/transcode/sess-1/master.m3u8",
      duration_seconds: 7200,
      player_start_seconds: 0,
      timeline_offset_seconds: 0,
      can_seek_anywhere: true,
      ...overrides,
    }),
  };
}

const fetchMock = vi.fn();

function sentBodies(): TranscodeStartRequest[] {
  return fetchMock.mock.calls.map(([, init]) => JSON.parse((init as RequestInit).body as string));
}

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockImplementation(() => Promise.resolve(transcodeStartResponse()));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderQuality() {
  return renderHook(
    () =>
      useTranscodeQuality({
        sessionId: "sess-1",
        selectedVersion: version,
        versions: [version],
        playMethod: "remux",
        initialPosition: 0,
      }),
    { wrapper },
  );
}

describe("useTranscodeQuality", () => {
  // RESOLUTION_HEIGHT is a client-side allowlist, so a resolution the server
  // introduces later resolves to 0 there -- and a native height of 0 filters out
  // every rung, leaving the viewer no way to escape a stalling stream. The height
  // must come from the matching ladder rung instead.
  it("offers lower rungs for a resolution the client does not hardcode", async () => {
    const exotic: PlayerFileVersion = { ...version, resolution: "1440p" };
    const { result } = renderHook(
      () =>
        useTranscodeQuality({
          sessionId: "sess-1",
          selectedVersion: exotic,
          versions: [exotic],
          playMethod: "remux",
          initialPosition: 0,
        }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.qualityOptions.length).toBeGreaterThan(2));
    const ids = result.current.qualityOptions.map((option) => option.id);
    expect(ids).toContain("1080p");
    expect(ids).toContain("720p");
    // Nothing at or above the source: 1440p sits between the 2160p and 1080p rungs.
    expect(ids).not.toContain("2160p");
  });

  it("adopts an audio-switch transport without starting another transcode", async () => {
    const { result } = renderHook(
      () =>
        useTranscodeQuality({
          sessionId: "sess-1",
          selectedVersion: version,
          versions: [version],
          playMethod: "remux",
          initialPosition: 100,
          transportRestart: {
            revision: 1,
            streamUrl: "/api/v1/playback/transcode/sess-1/master.m3u8?st=token",
            playerStartSeconds: 4,
            streamOriginSeconds: 96,
            canSeekAnywhere: false,
          },
        }),
      { wrapper },
    );

    await waitFor(() =>
      expect(result.current.transcodeStreamUrl).toContain("master.m3u8?st=token"),
    );
    expect(result.current.playerStartSeconds).toBe(4);
    expect(result.current.streamOriginSeconds).toBe(96);
    expect(result.current.canSeekAnywhere).toBe(false);
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("preserves video copy when auto-starting a resumed remux", async () => {
    fetchMock.mockImplementationOnce(() =>
      Promise.resolve(
        transcodeStartResponse({
          player_start_seconds: 2.261,
          stream_origin_seconds: 16,
          timeline_offset_seconds: 16,
          can_seek_anywhere: false,
        }),
      ),
    );
    const { result } = renderHook(
      () =>
        useTranscodeQuality({
          sessionId: "sess-1",
          selectedVersion: version,
          versions: [version],
          playMethod: "remux",
          initialPosition: 478.25,
        }),
      { wrapper },
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const body = sentBodies()[0]!;
    expect(body.seek_seconds).toBe(478.25);
    expect(body.target_codec_video).toBe("copy");
    expect(body.target_resolution).toBe("");
    expect(body.target_bitrate_kbps).toBe(0);
    await waitFor(() => expect(result.current.streamOriginSeconds).toBe(16));
    expect(result.current.playerStartSeconds).toBe(2.261);
    expect(result.current.canSeekAnywhere).toBe(false);
  });

  it("coalesces same-tick restarts into a single start with the final params", async () => {
    const { result } = renderQuality();

    // Mount auto-start has fired but its dispatch is macrotask-deferred. A
    // persisted bitmap subtitle selection lands in the same tick (subtitle
    // auto-selection on session start) — the two must collapse into ONE
    // server call carrying the burn-in, instead of spawning an ffmpeg that
    // is killed milliseconds later by the second start.
    act(() => {
      result.current.setSubtitleBurnIn(3, 0, 42);
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    // Give a (wrongly) surviving first dispatch a chance to fire.
    await new Promise((r) => setTimeout(r, 20));

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const body = sentBodies()[0]!;
    expect(body.subtitle_burn_in).toBe(true);
    expect(body.subtitle_track_index).toBe(3);
    expect(body.subtitle_media_file_id).toBe(42);
    // Burn-in composites into the frames, so codec copy must be off — the
    // server picks the encode codec from client ∩ encodable capabilities.
    expect(body.target_codec_video).toBeUndefined();
  });

  it("preserves a pending quality when burn-in is selected in the same tick", async () => {
    const { result } = renderQuality();

    act(() => {
      result.current.switchQuality("720p", 30);
      result.current.setSubtitleBurnIn(3, 30, 42);
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    await new Promise((r) => setTimeout(r, 20));

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const body = sentBodies()[0]!;
    expect(body.target_resolution).toBe("720p");
    expect(body.subtitle_burn_in).toBe(true);
    expect(body.subtitle_track_index).toBe(3);
    expect(body.subtitle_media_file_id).toBe(42);
  });

  it("still dispatches later restarts separately", async () => {
    const { result } = renderQuality();

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(sentBodies()[0]!.subtitle_burn_in).toBe(false);
    expect(sentBodies()[0]!.subtitle_media_file_id).toBeUndefined();

    // A user toggle in a later tick is a genuine restart, not coalesced away.
    act(() => {
      result.current.setSubtitleBurnIn(3, 120, 42);
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const second = sentBodies()[1]!;
    expect(second.subtitle_burn_in).toBe(true);
    expect(second.subtitle_track_index).toBe(3);
    expect(second.subtitle_media_file_id).toBe(42);
    expect(second.seek_seconds).toBe(120);
  });

  it("drops a deferred dispatch when the hook unmounts first", async () => {
    const { unmount } = renderQuality();

    // Auto-start is deferred; unmounting (exit → session DELETE) must cancel
    // it so no stray transcode/start resurrects the dead session.
    unmount();

    await new Promise((r) => setTimeout(r, 20));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("cancels an in-flight startup before a manifest arrives", async () => {
    let requestSignal: AbortSignal | undefined;
    fetchMock.mockImplementationOnce((_, init: RequestInit) => {
      requestSignal = init.signal as AbortSignal;
      return new Promise((_, reject) => {
        requestSignal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        );
      });
    });

    const { result } = renderQuality();

    await waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
    expect(result.current.startupGeneration).toBe(1);
    expect(result.current.isTranscoding).toBe(true);

    act(() => {
      result.current.cancelPendingTranscodeStart();
    });

    expect(requestSignal?.aborted).toBe(true);
    await waitFor(() => expect(result.current.isTranscoding).toBe(false));
    expect(result.current.error).toBeNull();
  });

  it("rolls back a failed burn-in selection so the same track can be retried", async () => {
    const { result } = renderQuality();

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    fetchMock.mockRejectedValueOnce(new Error("transcode failed"));

    act(() => {
      result.current.setSubtitleBurnIn(3, 120, 42);
    });

    await waitFor(() => expect(result.current.error).toMatch(/^Couldn't switch to Original/));
    expect(result.current.burnInSubtitleIndex).toBeNull();

    fetchMock.mockResolvedValueOnce(transcodeStartResponse());
    act(() => {
      result.current.setSubtitleBurnIn(3, 120, 42);
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    expect(sentBodies()[2]!.subtitle_track_index).toBe(3);
    expect(sentBodies()[2]!.subtitle_burn_in).toBe(true);
  });
});
