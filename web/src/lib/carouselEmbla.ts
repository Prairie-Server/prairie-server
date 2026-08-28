import type { EmblaOptionsType } from "embla-carousel";

const CAROUSEL_EMBLA_OPTIONS: EmblaOptionsType = {
  align: "start",
  containScroll: "trimSnaps",
  dragFree: true,
  slidesToScroll: "auto",
};

export function getCarouselEmblaOptions(
  overrides: EmblaOptionsType = {},
): EmblaOptionsType {
  return {
    ...CAROUSEL_EMBLA_OPTIONS,
    ...overrides,
  };
}

export function getCarouselWheelGestureOptions(target?: Element | null) {
  return {
    forceWheelAxis: "x" as const,
    target: target ?? undefined,
  };
}
