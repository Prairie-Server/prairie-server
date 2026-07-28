import { describe, expect, it } from "vitest";
import type { ClientCodecCapabilities } from "@/player/types";
import { buildLiveTVCapabilities } from "./liveTVCapabilities";

function caps(overrides: Partial<ClientCodecCapabilities> = {}): ClientCodecCapabilities {
  return {
    codecs_video: ["h264"],
    codecs_audio: ["aac"],
    containers: ["mp4"],
    max_resolution: "1080p",
    hdr: false,
    ...overrides,
  };
}

describe("buildLiveTVCapabilities", () => {
  // Chrome and Edge report AC-3 as decodable, but hls.js cannot lift it out of
  // the MPEG-TS segments the live bridge produces, so claiming it made the
  // server copy broadcast audio and playback stalled with no sound.
  it("drops audio codecs hls.js cannot transmux from MPEG-TS", () => {
    const payload = buildLiveTVCapabilities(
      caps({ codecs_audio: ["aac", "ac3", "eac3", "opus", "flac"] }),
    );

    expect(payload.codecs_audio).toEqual(["aac"]);
  });

  it("matches codec names case-insensitively and ignores stray whitespace", () => {
    const payload = buildLiveTVCapabilities(caps({ codecs_audio: [" AAC ", "AC3", "MP3"] }));

    expect(payload.codecs_audio).toEqual([" AAC ", "MP3"]);
  });

  it("passes video codecs and the resolution cap through untouched", () => {
    const payload = buildLiveTVCapabilities(
      caps({ codecs_video: ["h264", "hevc", "av1"], max_resolution: "720p" }),
    );

    expect(payload.codecs_video).toEqual(["h264", "hevc", "av1"]);
    expect(payload.max_resolution).toBe("720p");
  });

  // An empty audio list still leaves the video list declared, so the server
  // plans a transcode rather than falling back to copy.
  it("can end up with no usable audio codec", () => {
    const payload = buildLiveTVCapabilities(caps({ codecs_audio: ["ac3"] }));

    expect(payload.codecs_audio).toEqual([]);
    expect(payload.codecs_video.length).toBeGreaterThan(0);
  });
});
