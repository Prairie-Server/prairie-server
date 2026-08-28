import { describe, expect, it } from "vitest";
import { buildPlayerStreamUrl, joinApiStreamPath } from "./stream-url";

describe("joinApiStreamPath", () => {
  it("does not double-prefix when the path already includes /api/", () => {
    expect(joinApiStreamPath("/api/v1", "/api/v1/playback/transcode/s/master.m3u8")).toBe(
      "/api/v1/playback/transcode/s/master.m3u8",
    );
  });

  it("still prefixes legacy bare playback paths with the API mount", () => {
    expect(joinApiStreamPath("/api/v1", "/playback/transcode/s/master.m3u8")).toBe(
      "/api/v1/playback/transcode/s/master.m3u8",
    );
  });

  it("joins an absolute origin with an already-prefixed API path", () => {
    expect(joinApiStreamPath("https://api.example.com", "/api/v1/stream/abc")).toBe(
      "https://api.example.com/api/v1/stream/abc",
    );
  });
});

describe("buildPlayerStreamUrl", () => {
  it("joins the access token with `&` when the stream path already has `?st=`", () => {
    const url = buildPlayerStreamUrl(
      "https://api.example.com",
      "/api/v1/playback/stream/abc.m3u8?st=streamtoken123",
      "jwt-access-token",
    );

    const parsed = new URL(url);
    // Both params must survive as separate query keys.
    expect(parsed.searchParams.get("st")).toBe("streamtoken123");
    expect(parsed.searchParams.get("token")).toBe("jwt-access-token");
  });

  it("uses `?` when the stream path has no existing query string", () => {
    const url = buildPlayerStreamUrl(
      "https://api.example.com",
      "/api/v1/playback/stream/abc.m3u8",
      "jwt-access-token",
    );

    expect(url).toBe(
      "https://api.example.com/api/v1/playback/stream/abc.m3u8?token=jwt-access-token",
    );
    const parsed = new URL(url);
    expect(parsed.searchParams.get("token")).toBe("jwt-access-token");
  });

  it("preserves a server-anchored seek param instead of synthesizing one", () => {
    // v3 plans arrive fully anchored: the seek offset is the server's decision
    // and rides in the plan's stream URL. The helper must pass it through
    // untouched and never add one of its own.
    const url = buildPlayerStreamUrl(
      "https://api.example.com",
      "/api/v1/playback/stream/abc.m3u8?st=streamtoken123&seek=12.500",
      "jwt-access-token",
    );

    const parsed = new URL(url);
    expect(parsed.searchParams.get("st")).toBe("streamtoken123");
    expect(parsed.searchParams.get("token")).toBe("jwt-access-token");
    expect(parsed.searchParams.get("seek")).toBe("12.500");
  });

  it("returns the path unchanged when there is no token", () => {
    const url = buildPlayerStreamUrl(
      "https://api.example.com",
      "/api/v1/playback/proxy/sometoken/abc.m3u8",
      null,
    );

    expect(url).toBe("https://api.example.com/api/v1/playback/proxy/sometoken/abc.m3u8");
  });
});
