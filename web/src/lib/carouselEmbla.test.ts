import { describe, expect, it } from "vitest";
import { getCarouselEmblaOptions, getCarouselWheelGestureOptions } from "./carouselEmbla";

describe("carouselEmbla", () => {
  it("enables drag-free momentum with trimmed edge scrolling", () => {
    expect(getCarouselEmblaOptions()).toMatchObject({
      align: "start",
      containScroll: "trimSnaps",
      dragFree: true,
    });
  });

  it("forces wheel gestures onto the horizontal axis", () => {
    const target = { nodeName: "DIV" } as unknown as HTMLElement;

    expect(getCarouselWheelGestureOptions(target)).toMatchObject({
      forceWheelAxis: "x",
      target,
    });
  });

  it("omits target when none is provided and merges Embla overrides", () => {
    expect(getCarouselWheelGestureOptions()).toEqual({
      forceWheelAxis: "x",
      target: undefined,
    });
    expect(getCarouselWheelGestureOptions(null)).toEqual({
      forceWheelAxis: "x",
      target: undefined,
    });
    expect(getCarouselEmblaOptions({ loop: true, dragFree: false })).toMatchObject({
      align: "start",
      loop: true,
      dragFree: false,
    });
  });
});
