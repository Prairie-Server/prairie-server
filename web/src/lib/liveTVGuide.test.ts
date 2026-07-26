import { describe, expect, it } from "vitest";

import { channelDisplayNumber, channelLabel, formatGuideTime, pickNowNext } from "./liveTVGuide";

describe("liveTVGuide", () => {
  it("prefers number overrides", () => {
    expect(channelDisplayNumber({ number: "5.1", number_override: " 99.1 " })).toBe("99.1");
    expect(channelDisplayNumber({ number: "5.1" })).toBe("5.1");
    expect(channelLabel({ number: "5.1", callsign: "KING", name: "King" })).toBe("5.1 · KING");
  });

  it("picks now and next programmes for a channel", () => {
    const now = new Date("2026-07-25T19:30:00Z");
    const result = pickNowNext(
      [
        {
          id: "a",
          channel_id: "ch1",
          title: "Earlier",
          start: "2026-07-25T18:00:00Z",
          stop: "2026-07-25T19:00:00Z",
        },
        {
          id: "b",
          channel_id: "ch1",
          title: "News",
          start: "2026-07-25T19:00:00Z",
          stop: "2026-07-25T20:00:00Z",
        },
        {
          id: "c",
          channel_id: "ch1",
          title: "Drama",
          start: "2026-07-25T20:00:00Z",
          stop: "2026-07-25T21:00:00Z",
        },
        {
          id: "x",
          channel_id: "ch2",
          title: "Other",
          start: "2026-07-25T19:00:00Z",
          stop: "2026-07-25T20:00:00Z",
        },
      ],
      "ch1",
      now,
    );
    expect(result.now?.title).toBe("News");
    expect(result.next?.title).toBe("Drama");
  });

  it("picks next when only current is airing at end of list", () => {
    const now = new Date("2026-07-25T19:30:00Z");
    const result = pickNowNext(
      [
        {
          id: "only",
          channel_id: "ch1",
          title: "Solo",
          start: "2026-07-25T19:00:00Z",
          stop: "2026-07-25T20:00:00Z",
        },
      ],
      "ch1",
      now,
    );
    expect(result.now?.title).toBe("Solo");
    expect(result.next).toBeNull();
  });

  it("skips invalid timestamps", () => {
    const result = pickNowNext(
      [
        {
          id: "bad",
          channel_id: "ch1",
          title: "Bad",
          start: "nope",
          stop: "still-nope",
        },
      ],
      "ch1",
      new Date("2026-07-25T19:30:00Z"),
    );
    expect(result.now).toBeNull();
    expect(result.next).toBeNull();
  });

  it("formats guide times", () => {
    expect(formatGuideTime("not-a-date")).toBe("");
    expect(formatGuideTime("2026-07-25T19:05:00Z")).toMatch(/\d/);
  });
});
