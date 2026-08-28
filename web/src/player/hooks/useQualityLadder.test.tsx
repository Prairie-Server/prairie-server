import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PlayerConfigProvider } from "../context/PlayerConfigContext";
import { __resetQualityLadderCache, useQualityLadder } from "./useQualityLadder";

const playerFetch = vi.fn();
vi.mock("../player-fetch", () => ({
  PlayerFetchError: class extends Error {},
  playerFetch: (...args: unknown[]) => playerFetch(...args),
}));

const config = {
  apiBaseUrl: "http://server/api/v1",
  getAccessToken: () => "tok",
};

function wrapper({ children }: { children: ReactNode }) {
  return <PlayerConfigProvider config={config as never}>{children}</PlayerConfigProvider>;
}

function render() {
  return renderHook(() => useQualityLadder(), { wrapper });
}

describe("useQualityLadder", () => {
  beforeEach(() => {
    playerFetch.mockReset();
    __resetQualityLadderCache();
  });

  it("serves the server ladder once loaded", async () => {
    playerFetch.mockResolvedValue({
      rungs: [
        { id: "1080p", label: "1080p", resolution: "1080p", height: 1080, bitrate_kbps: 6000 },
        { id: "540p", label: "540p", resolution: "540p", height: 540, bitrate_kbps: 1800 },
      ],
      modes: ["auto", "original"],
    });

    const { result } = render();
    await waitFor(() => expect(result.current).toHaveLength(2));
    // A rung the client has never heard of must come through untouched -- that is
    // the point of deriving from the server rather than hardcoding.
    expect(result.current.map((rung) => rung.id)).toEqual(["1080p", "540p"]);
    expect(result.current[1]?.bitrate_kbps).toBe(1800);
  });

  // An empty quality menu would strip a viewer's ability to escape a stalling
  // stream, so a failure must degrade to the previous hardcoded behaviour.
  it("falls back when the request fails", async () => {
    playerFetch.mockRejectedValue(new Error("offline"));

    const { result } = render();
    await waitFor(() => expect(result.current.length).toBeGreaterThan(0));
    expect(result.current.some((rung) => rung.id === "1080p")).toBe(true);
  });

  it("falls back when the server returns an empty or malformed ladder", async () => {
    playerFetch.mockResolvedValue({ rungs: [], modes: [] });

    const { result } = render();
    await waitFor(() => expect(result.current.length).toBeGreaterThan(0));
    expect(result.current.some((rung) => rung.id === "720p")).toBe(true);
  });

  // All-or-nothing: a partially valid response means the contract shifted, and
  // serving the survivors would hide that behind a truncated menu.
  it("rejects a ladder with any incomplete rung", async () => {
    playerFetch.mockResolvedValue({
      rungs: [
        { id: "1080p", label: "1080p", resolution: "1080p", height: 1080, bitrate_kbps: 6000 },
        { id: "720p", label: "720p", resolution: "720p", height: 720 },
      ],
      modes: ["auto"],
    });

    const { result } = render();
    await waitFor(() => expect(result.current.length).toBeGreaterThan(0));
    // The fallback has 7 rungs; a two-rung response was not adopted.
    expect(result.current.length).toBeGreaterThan(2);
  });

  // Caching the fallback after an invalid response would make every later mount
  // skip the server for the rest of the page's lifetime.
  it("does not cache the fallback after an invalid response", async () => {
    playerFetch.mockResolvedValueOnce({ rungs: [], modes: [] });
    const first = render();
    await waitFor(() => expect(first.result.current.length).toBeGreaterThan(0));
    first.unmount();

    playerFetch.mockResolvedValue({
      rungs: [{ id: "540p", label: "540p", resolution: "540p", height: 540, bitrate_kbps: 1800 }],
      modes: ["auto"],
    });
    const second = render();
    await waitFor(() => expect(second.result.current).toHaveLength(1));
    expect(second.result.current[0]?.id).toBe("540p");
  });

  it("never renders an empty ladder, even on the first frame", () => {
    playerFetch.mockReturnValue(new Promise(() => {}));

    const { result } = render();
    expect(result.current.length).toBeGreaterThan(0);
  });

  // The ladder is server configuration, not per-session, so mounting the player
  // repeatedly must not re-request it.
  it("caches across mounts", async () => {
    playerFetch.mockResolvedValue({
      rungs: [{ id: "720p", label: "720p", resolution: "720p", height: 720, bitrate_kbps: 2000 }],
      modes: ["auto"],
    });

    const first = render();
    await waitFor(() => expect(first.result.current).toHaveLength(1));
    first.unmount();

    const second = render();
    await waitFor(() => expect(second.result.current).toHaveLength(1));
    expect(playerFetch).toHaveBeenCalledTimes(1);
  });

  // A transient error must not pin the fallback for the page's lifetime.
  it("retries after a failure instead of caching it", async () => {
    playerFetch.mockRejectedValueOnce(new Error("flaky"));
    const first = render();
    await waitFor(() => expect(first.result.current.length).toBeGreaterThan(0));
    first.unmount();

    playerFetch.mockResolvedValue({
      rungs: [{ id: "480p", label: "480p", resolution: "480p", height: 480, bitrate_kbps: 1500 }],
      modes: ["auto"],
    });
    const second = render();
    await waitFor(() => expect(second.result.current).toHaveLength(1));
    expect(second.result.current[0]?.id).toBe("480p");
  });
});
