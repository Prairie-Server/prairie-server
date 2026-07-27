import { describe, expect, it } from "vitest";
import {
  buildGuideWindow,
  channelDisplayNumber,
  channelLabel,
  guideTimeTicks,
  layoutProgramsForChannel,
  pickNowNext,
  progressFraction,
} from "./liveTVGuide";

describe("liveTVGuide helpers", () => {
  it("prefers number_override for display", () => {
    expect(channelDisplayNumber({ number: "5.1", number_override: "5" })).toBe("5");
    expect(channelLabel({ number: "5.1", callsign: "KING", name: "NBC" })).toBe("5.1 · KING");
  });

  it("picks now and next programmes", () => {
    const now = new Date("2026-07-27T15:00:00Z");
    const result = pickNowNext(
      [
        {
          id: "a",
          channel_id: "ch1",
          title: "Past",
          start: "2026-07-27T13:00:00Z",
          stop: "2026-07-27T14:00:00Z",
        },
        {
          id: "b",
          channel_id: "ch1",
          title: "Now Show",
          start: "2026-07-27T14:30:00Z",
          stop: "2026-07-27T15:30:00Z",
        },
        {
          id: "c",
          channel_id: "ch1",
          title: "Next Show",
          start: "2026-07-27T15:30:00Z",
          stop: "2026-07-27T16:30:00Z",
        },
      ],
      "ch1",
      now,
    );
    expect(result.now?.title).toBe("Now Show");
    expect(result.next?.title).toBe("Next Show");
  });

  it("layouts programs inside the guide window", () => {
    const now = new Date("2026-07-27T15:00:00Z");
    const window = buildGuideWindow(now, 0.5, 1.5);
    expect(guideTimeTicks(window).length).toBeGreaterThan(0);
    const laid = layoutProgramsForChannel(
      [
        {
          id: "b",
          channel_id: "ch1",
          title: "Now Show",
          start: "2026-07-27T14:30:00Z",
          stop: "2026-07-27T15:30:00Z",
        },
      ],
      "ch1",
      window,
      now,
    );
    expect(laid).toHaveLength(1);
    expect(laid[0]?.isNow).toBe(true);
    expect(laid[0]?.widthPx).toBeGreaterThan(40);
  });

  it("computes progress fraction", () => {
    const now = new Date("2026-07-27T15:00:00Z");
    expect(progressFraction("2026-07-27T14:00:00Z", "2026-07-27T16:00:00Z", now)).toBeCloseTo(0.5);
  });
});
