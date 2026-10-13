import { afterEach, describe, expect, it, vi } from "vitest";
import { buildLiveWatchHref, releaseLiveTVSessionOnUnload } from "./liveTVWatch";

const mockV2 = vi.fn();
vi.mock("@/api/v2/request", () => ({
  v2: (...args: unknown[]) => mockV2(...args),
}));

afterEach(() => {
  vi.restoreAllMocks();
});

describe("buildLiveWatchHref", () => {
  it("builds a fullscreen watch path for the channel", () => {
    expect(buildLiveWatchHref("ch-1")).toBe("/watch/live/ch-1?return=%2Flivetv");
  });

  it("encodes channel ids and custom return paths", () => {
    expect(buildLiveWatchHref("a/b", "/")).toBe("/watch/live/a%2Fb?return=%2F");
    expect(buildLiveWatchHref("ch-2", "/home")).toBe("/watch/live/ch-2?return=%2Fhome");
  });
});

describe("releaseLiveTVSessionOnUnload", () => {
  it("sends a keepalive v2 DELETE so the tuner is freed", () => {
    mockV2.mockReset().mockResolvedValue(undefined);

    releaseLiveTVSessionOnUnload("sess-1");

    expect(mockV2).toHaveBeenCalledTimes(1);
    expect(mockV2).toHaveBeenCalledWith("DELETE /api/v2/livetv/sessions/{session_id}", {
      path: { session_id: "sess-1" },
      keepalive: true,
      retryAuthentication: false,
    });
  });

  it("ignores an empty session id", () => {
    mockV2.mockReset();

    releaseLiveTVSessionOnUnload("");

    expect(mockV2).not.toHaveBeenCalled();
  });
});
