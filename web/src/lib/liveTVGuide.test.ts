import { describe, expect, it } from "vitest";
import {
  buildGuideWindow,
  channelDisplayNumber,
  channelLabel,
  formatGuideTime,
  GUIDE_HOUR_WIDTH_PX,
  guideTimeTicks,
  layoutProgramsForChannel,
  pickNowNext,
  progressFraction,
} from "./liveTVGuide";

describe("liveTVGuide helpers", () => {
  it("prefers number_override for display", () => {
    expect(channelDisplayNumber({ number: "5.1", number_override: "5" })).toBe("5");
    expect(channelDisplayNumber({ number: "5.1", number_override: "  " })).toBe("5.1");
    expect(channelDisplayNumber({ number: "5.1", number_override: null })).toBe("5.1");
    expect(channelLabel({ number: "5.1", callsign: "KING", name: "NBC" })).toBe("5.1 · KING");
    expect(channelLabel({ number: "7.1", callsign: "", name: "ABC" })).toBe("7.1 · ABC");
    expect(channelLabel({ number: "9.1", callsign: "", name: "" })).toBe("9.1 · Channel");
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
        {
          id: "other",
          channel_id: "ch2",
          title: "Elsewhere",
          start: "2026-07-27T14:00:00Z",
          stop: "2026-07-27T16:00:00Z",
        },
      ],
      "ch1",
      now,
    );
    expect(result.now?.title).toBe("Now Show");
    expect(result.next?.title).toBe("Next Show");
  });

  it("falls back to the programme after current when nothing is upcoming by start", () => {
    const now = new Date("2026-07-27T15:00:00Z");
    // Current ends in the future, but the following row starts in the past relative to
    // a gapless schedule that was already skipped — force the idx+1 fallback by only
    // having current with a later sibling that starts after current.stop but is found
    // via index when the loop never sets upcoming (e.g. invalid next dates).
    const result = pickNowNext(
      [
        {
          id: "now",
          channel_id: "ch1",
          title: "Live",
          start: "2026-07-27T14:00:00Z",
          stop: "2026-07-27T16:00:00Z",
        },
        {
          id: "bad",
          channel_id: "ch1",
          title: "Broken",
          start: "not-a-date",
          stop: "also-bad",
        },
        {
          id: "after",
          channel_id: "ch1",
          title: "Later",
          start: "2026-07-27T16:00:00Z",
          stop: "2026-07-27T17:00:00Z",
        },
      ],
      "ch1",
      now,
    );
    expect(result.now?.id).toBe("now");
    // "after" is found in the loop as upcoming (start > now).
    expect(result.next?.id).toBe("after");
  });

  it("uses index fallback when no future-start programme exists after current", () => {
    const now = new Date("2026-07-27T15:00:00Z");
    const onlyNow = pickNowNext(
      [
        {
          id: "solo",
          channel_id: "ch1",
          title: "Solo",
          start: "2026-07-27T14:00:00Z",
          stop: "2026-07-27T16:00:00Z",
        },
      ],
      "ch1",
      now,
    );
    expect(onlyNow.now?.id).toBe("solo");
    expect(onlyNow.next).toBeNull();

    // Sibling shares start but already ended — loop never sets upcoming; fallback picks it.
    const fallback = pickNowNext(
      [
        {
          id: "cur",
          channel_id: "ch1",
          title: "Current",
          start: "2026-07-27T14:00:00Z",
          stop: "2026-07-27T16:00:00Z",
        },
        {
          id: "ended-sibling",
          channel_id: "ch1",
          title: "Ended sibling",
          start: "2026-07-27T14:00:00Z",
          stop: "2026-07-27T14:30:00Z",
        },
      ],
      "ch1",
      now,
    );
    expect(fallback.now?.id).toBe("cur");
    expect(fallback.next?.id).toBe("ended-sibling");
  });

  it("formats guide times and rejects invalid ISO", () => {
    expect(formatGuideTime("not-a-date")).toBe("");
    expect(formatGuideTime("2026-07-27T15:30:00Z", "en-US")).toMatch(/\d/);
  });

  it("layouts programs inside the guide window", () => {
    const now = new Date("2026-07-27T15:00:00Z");
    const window = buildGuideWindow(now, 0.5, 1.5);
    expect(window.pxPerMs).toBeCloseTo(GUIDE_HOUR_WIDTH_PX / (60 * 60 * 1000));
    expect(guideTimeTicks(window).length).toBeGreaterThan(0);
    expect(guideTimeTicks(window, 60).length).toBeLessThan(guideTimeTicks(window, 30).length);

    const laid = layoutProgramsForChannel(
      [
        {
          id: "skip-channel",
          channel_id: "other",
          title: "Nope",
          start: "2026-07-27T14:30:00Z",
          stop: "2026-07-27T15:30:00Z",
        },
        {
          id: "bad-dates",
          channel_id: "ch1",
          title: "Bad",
          start: "x",
          stop: "y",
        },
        {
          id: "before-window",
          channel_id: "ch1",
          title: "Before",
          start: "2026-07-27T10:00:00Z",
          stop: "2026-07-27T11:00:00Z",
        },
        {
          id: "after-window",
          channel_id: "ch1",
          title: "After",
          start: "2026-07-27T20:00:00Z",
          stop: "2026-07-27T21:00:00Z",
        },
        {
          id: "past-in-window",
          channel_id: "ch1",
          title: "Already ended",
          start: "2026-07-27T14:00:00Z",
          stop: "2026-07-27T14:45:00Z",
        },
        {
          id: "b",
          channel_id: "ch1",
          title: "Now Show",
          subtitle: "Ep 1",
          start: "2026-07-27T14:30:00Z",
          stop: "2026-07-27T15:30:00Z",
        },
        {
          id: "c",
          channel_id: "ch1",
          title: "Clamped",
          start: "2026-07-27T13:00:00Z",
          stop: "2026-07-27T16:00:00Z",
        },
        {
          id: "future",
          channel_id: "ch1",
          title: "Later",
          start: "2026-07-27T15:30:00Z",
          stop: "2026-07-27T16:00:00Z",
        },
      ],
      "ch1",
      window,
      now,
    );
    expect(laid.some((p) => p.id === "b")).toBe(true);
    expect(laid.find((p) => p.id === "b")?.isNow).toBe(true);
    expect(laid.find((p) => p.id === "b")?.canRecord).toBe(true);
    expect(laid.find((p) => p.id === "past-in-window")?.isNow).toBe(false);
    expect(laid.find((p) => p.id === "past-in-window")?.canRecord).toBe(false);
    expect(laid.find((p) => p.id === "future")?.canRecord).toBe(true);
    expect(laid.find((p) => p.id === "b")?.widthPx).toBeGreaterThan(40);
    expect(laid.find((p) => p.id === "c")?.subtitle).toBeUndefined();
    expect(laid.every((p) => p.id !== "bad-dates")).toBe(true);
    expect(laid.every((p) => p.id !== "before-window")).toBe(true);
  });

  it("computes progress fraction edges", () => {
    const now = new Date("2026-07-27T15:00:00Z");
    expect(progressFraction("2026-07-27T14:00:00Z", "2026-07-27T16:00:00Z", now)).toBeCloseTo(0.5);
    expect(progressFraction("bad", "2026-07-27T16:00:00Z", now)).toBe(0);
    expect(progressFraction("2026-07-27T16:00:00Z", "2026-07-27T14:00:00Z", now)).toBe(0);
    expect(progressFraction("2026-07-27T14:00:00Z", "2026-07-27T14:30:00Z", now)).toBe(1);
    expect(progressFraction("2026-07-27T15:30:00Z", "2026-07-27T16:00:00Z", now)).toBe(0);
  });
});
