import { describe, expect, it } from "vitest";
import { buildLiveWatchHref } from "./liveTVWatch";

describe("buildLiveWatchHref", () => {
  it("builds a fullscreen watch path for the channel", () => {
    expect(buildLiveWatchHref("ch-1")).toBe("/watch/live/ch-1?return=%2Flivetv");
  });

  it("encodes channel ids and custom return paths", () => {
    expect(buildLiveWatchHref("a/b", "/")).toBe("/watch/live/a%2Fb?return=%2F");
    expect(buildLiveWatchHref("ch-2", "/home")).toBe("/watch/live/ch-2?return=%2Fhome");
  });
});
