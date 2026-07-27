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
    expect(
      joinApiStreamPath("https://api.example.com", "/api/v1/stream/abc"),
    ).toBe("https://api.example.com/api/v1/stream/abc");
  });
});

describe("buildPlayerStreamUrl", () => {
  it("joins the access token with `&` when the stream path already has `?st=`", () => {
    const url = buildPlayerStreamUrl(
      "https://api.example.com",
      "/api/v1/playback/stream/abc.m3u8?st=streamtoken123",
      "jwt-access-token",
      "direct",
      0,
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
      "direct",
      0,
    );

    expect(url).toBe(
      "https://api.example.com/api/v1/playback/stream/abc.m3u8?token=jwt-access-token",
    );
    const parsed = new URL(url);
    expect(parsed.searchParams.get("token")).toBe("jwt-access-token");
  });

  it("adds a seek param for remux without clobbering an existing `?st=`", () => {
    const url = buildPlayerStreamUrl(
      "https://api.example.com",
      "/api/v1/playback/stream/abc.m3u8?st=streamtoken123",
      "jwt-access-token",
      "remux",
      12.5,
    );

    const parsed = new URL(url);
    expect(parsed.searchParams.get("st")).toBe("streamtoken123");
    expect(parsed.searchParams.get("token")).toBe("jwt-access-token");
    expect(parsed.searchParams.get("seek")).toBe("12.500");
  });

  it("returns the path unchanged when there is no token or extra params", () => {
    const url = buildPlayerStreamUrl(
      "https://api.example.com",
      "/api/v1/playback/proxy/sometoken/abc.m3u8",
      null,
      "direct",
      0,
    );

    expect(url).toBe("https://api.example.com/api/v1/playback/proxy/sometoken/abc.m3u8");
  });
});
