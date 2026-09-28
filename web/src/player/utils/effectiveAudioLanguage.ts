interface AudioLanguageTrack {
  language?: string;
}

interface AudioLanguageVersion {
  audio_tracks?: AudioLanguageTrack[];
  effective_audio_track_index?: number;
  effective_audio_language?: string;
}

function normalizeLanguage(language: string | null | undefined): string | null {
  const normalized = language?.trim();
  return normalized ? normalized : null;
}

export function resolveVersionAudioLanguage<T extends AudioLanguageVersion>(
  version: T | null | undefined,
  trackIndex: number | null | undefined,
): string | null {
  const trackLanguage =
    trackIndex != null ? normalizeLanguage(version?.audio_tracks?.[trackIndex]?.language) : null;
  if (trackLanguage) {
    return trackLanguage;
  }

  const effectiveTrackIndex = version?.effective_audio_track_index;
  if (trackIndex == null || effectiveTrackIndex == null || trackIndex !== effectiveTrackIndex) {
    return null;
  }

  return normalizeLanguage(version?.effective_audio_language);
}
