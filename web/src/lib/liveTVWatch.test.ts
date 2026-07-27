import { afterEach, describe, expect, it, vi } from "vitest";
import { buildLiveWatchHref, releaseLiveTVSessionOnUnload } from "./liveTVWatch";

vi.mock("@/api/client", () => ({
  getAccessToken: () => "access-token",
  getProfileId: () => "profile-1",
  getProfileToken: () => "profile-token",
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
  it("sends an authenticated keepalive DELETE so the tuner is freed", () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    releaseLiveTVSessionOnUnload("sess-1");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/livetv/sessions/sess-1");
    expect(init).toMatchObject({ method: "DELETE", keepalive: true });
    expect(init.headers).toMatchObject({
      Authorization: "Bearer access-token",
      "X-Profile-Id": "profile-1",
      "X-Profile-Token": "profile-token",
    });
  });

  it("ignores an empty session id", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    releaseLiveTVSessionOnUnload("");

    expect(fetchMock).not.toHaveBeenCalled();
  });
});
