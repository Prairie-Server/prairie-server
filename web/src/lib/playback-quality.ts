export type PlaybackQualityPreset = "any" | "standard" | "4k";

export const PLAYBACK_QUALITY_OPTIONS: Array<{
  value: PlaybackQualityPreset;
  label: string;
  description: string;
}> = [
  { value: "any", label: "Any", description: "Allow all resolutions" },
  {
    value: "standard",
    label: "Standard",
    description: "Hide 4K and higher versions",
  },
  { value: "4k", label: "4K", description: "Allow 4K and lower versions" },
];

export function canonicalPlaybackQuality(
  value: string | null | undefined,
): string {
  switch ((value ?? "").trim().toLowerCase()) {
    case "":
    case "any":
      return "";
    case "standard":
    case "480p":
    case "720p":
    case "1080p":
      return "1080p";
    case "4k":
    case "uhd":
    case "2160p":
    case "4320p":
      return "2160p";
    default:
      return "";
  }
}

export function playbackQualityPresetFromValue(
  value: string | null | undefined,
): PlaybackQualityPreset {
  switch (canonicalPlaybackQuality(value)) {
    case "2160p":
      return "4k";
    case "1080p":
      return "standard";
    default:
      return "any";
  }
}

export function playbackQualityValueFromPreset(
  preset: PlaybackQualityPreset,
): string {
  switch (preset) {
    case "standard":
      return "1080p";
    case "4k":
      return "2160p";
    default:
      return "";
  }
}

export function formatPlaybackQualityPreset(
  value: string | null | undefined,
): string {
  switch (playbackQualityPresetFromValue(value)) {
    case "standard":
      return "Standard";
    case "4k":
      return "4K";
    default:
      return "Any";
  }
}
