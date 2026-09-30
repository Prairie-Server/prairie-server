import { describe, expect, it } from "vitest";
import { describeMediaEvent, formatPlaybackEventTime, PlaybackEventLog } from "./playback-events";

describe("PlaybackEventLog", () => {
  it("keeps only the newest entries, oldest first", () => {
    const log = new PlaybackEventLog(3);
    for (let i = 1; i <= 5; i++) log.add(`event ${i}`, i * 10_000);
    expect(log.snapshot().map((entry) => entry.message)).toEqual(["event 3", "event 4", "event 5"]);
  });

  it("collapses a burst of the same event into one line", () => {
    const log = new PlaybackEventLog();
    log.add("buffering @ 1.0s", 1_000);
    log.add("buffering @ 1.0s", 1_500);
    log.add("buffering @ 1.0s", 5_000);
    expect(log.snapshot()).toEqual([
      { at: 1_500, message: "buffering @ 1.0s" },
      { at: 5_000, message: "buffering @ 1.0s" },
    ]);
  });

  it("ignores blank messages and can be cleared", () => {
    const log = new PlaybackEventLog();
    log.add("   ");
    expect(log.snapshot()).toEqual([]);
    log.add("playing");
    log.clear();
    expect(log.snapshot()).toEqual([]);
  });

  it("returns a snapshot that later adds do not mutate", () => {
    const log = new PlaybackEventLog();
    log.add("one", 1);
    const before = log.snapshot();
    log.add("two", 5_000);
    expect(before).toHaveLength(1);
  });
});

describe("formatPlaybackEventTime", () => {
  it("pads hours, minutes and seconds", () => {
    const at = new Date(2026, 0, 2, 3, 4, 5).getTime();
    expect(formatPlaybackEventTime(at)).toBe("03:04:05");
  });
});

describe("describeMediaEvent", () => {
  function fakeVideo(overrides: Partial<HTMLVideoElement> = {}): HTMLVideoElement {
    return {
      currentTime: 12.34,
      videoWidth: 1920,
      videoHeight: 1080,
      error: null,
      ...overrides,
    } as HTMLVideoElement;
  }

  it("describes playback state changes with the playhead", () => {
    expect(describeMediaEvent("waiting", fakeVideo())).toBe("buffering @ 12.3s");
    expect(describeMediaEvent("seeking", fakeVideo())).toBe("seeking to 12.3s");
    expect(describeMediaEvent("loadedmetadata", fakeVideo())).toBe("metadata 1920x1080");
  });

  it("names media errors", () => {
    const video = fakeVideo({ error: { code: 3, message: "PIPELINE_ERROR_DECODE" } as MediaError });
    expect(describeMediaEvent("error", video)).toBe("media error (decode): PIPELINE_ERROR_DECODE");
  });

  it("skips events it does not log", () => {
    expect(describeMediaEvent("timeupdate", fakeVideo())).toBeNull();
  });
});
