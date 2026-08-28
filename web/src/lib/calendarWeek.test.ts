import { describe, expect, it } from "vitest";

import {
  addDays,
  addWeeks,
  formatDayHeading,
  formatMonthYear,
  formatShortDay,
  formatWeekRangeLabel,
  getWeekDays,
  getWeekStart,
  isToday,
} from "./calendarWeek";

describe("calendarWeek", () => {
  it("adds days without rolling into the next week request", () => {
    expect(addDays("2026-04-06", 6)).toBe("2026-04-12");
  });

  it("still advances week starts by full weeks", () => {
    expect(addWeeks("2026-04-06", 1)).toBe("2026-04-13");
  });

  it("normalizes arbitrary dates to the Monday of their week", () => {
    expect(getWeekStart(new Date(2026, 3, 12))).toBe("2026-04-06"); // Sunday
    expect(getWeekStart(new Date(2026, 3, 6))).toBe("2026-04-06"); // Monday
    expect(getWeekStart(new Date(2026, 3, 8))).toBe("2026-04-06"); // Wednesday
  });

  it("lists seven ISO days for a Monday week start", () => {
    expect(getWeekDays("2026-04-06")).toEqual([
      "2026-04-06",
      "2026-04-07",
      "2026-04-08",
      "2026-04-09",
      "2026-04-10",
      "2026-04-11",
      "2026-04-12",
    ]);
  });

  it("formats day headings with ordinals", () => {
    expect(formatDayHeading("2026-04-07")).toMatch(/Tuesday, April 7th/);
    expect(formatDayHeading("2026-04-01")).toMatch(/Wednesday, April 1st/);
    expect(formatDayHeading("2026-04-02")).toMatch(/Thursday, April 2nd/);
    expect(formatDayHeading("2026-04-03")).toMatch(/Friday, April 3rd/);
    expect(formatDayHeading("2026-04-11")).toMatch(/Saturday, April 11th/);
  });

  it("formats short day labels", () => {
    expect(formatShortDay("2026-04-07")).toEqual({ label: expect.any(String), day: 7 });
    expect(formatShortDay("2026-04-07").label.length).toBeGreaterThan(0);
  });

  it("formats month/year and same-month week ranges", () => {
    expect(formatMonthYear("2026-04-07")).toMatch(/April 2026/);
    expect(formatWeekRangeLabel("2026-04-06")).toMatch(/Apr 6 – 12, 2026/);
  });

  it("formats week ranges that span two months", () => {
    // Monday 2026-03-30 → Sunday 2026-04-05
    expect(formatWeekRangeLabel("2026-03-30")).toMatch(/Mar 30/);
    expect(formatWeekRangeLabel("2026-03-30")).toMatch(/Apr 5/);
  });

  it("detects today against local YYYY-MM-DD", () => {
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, "0");
    const dd = String(now.getDate()).padStart(2, "0");
    expect(isToday(`${yyyy}-${mm}-${dd}`)).toBe(true);
    expect(isToday("1999-01-01")).toBe(false);
  });
});
