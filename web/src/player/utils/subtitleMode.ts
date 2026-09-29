import type { SubtitleMode } from "../types";

export function derivePersistedSubtitleMode(index: number | null): SubtitleMode {
  return index === null ? "off" : "always";
}

export function normalizeSubtitleMode(mode: string | null | undefined): SubtitleMode {
  switch (mode) {
    case "off":
    case "auto":
    case "always":
      return mode;
    default:
      return "auto";
  }
}
