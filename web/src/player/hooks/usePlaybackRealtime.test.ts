import { describe, expect, it } from "vitest";
import { createPlaybackRealtimeUrlFactory } from "./usePlaybackRealtime";

describe("createPlaybackRealtimeUrlFactory", () => {
  it("reads the current token for each reconnect attempt", () => {
    let token: string | null = "stale-token";
    const getUrl = createPlaybackRealtimeUrlFactory(
      "/api/v1",
      "session-123",
      () => token,
    );

    expect(getUrl()).toBe(
      "/api/v1/playback/sessions/session-123/control/ws?token=stale-token",
    );

    token = "fresh-token";

    expect(getUrl()).toBe(
      "/api/v1/playback/sessions/session-123/control/ws?token=fresh-token",
    );
  });
});
