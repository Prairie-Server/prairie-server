import type { EpisodeRef } from "../types";

interface NextEpisodeOverlayProps {
  episode: EpisodeRef;
  secondsRemaining: number;
  onSkip: () => void;
  onCancel: () => void;
}

/**
 * Overlay shown during the credits region with a countdown to the next episode.
 */
export function NextEpisodeOverlay({
  episode,
  secondsRemaining,
  onSkip,
  onCancel,
}: NextEpisodeOverlayProps) {
  return (
    <div className="absolute right-3 bottom-24 left-3 z-50 flex max-w-sm flex-col gap-2 rounded-lg bg-black/80 p-4 text-white sm:right-6 sm:left-auto sm:min-w-[260px]">
      <div className="text-xs tracking-wider text-white/60 uppercase">
        Up Next in {secondsRemaining}s
      </div>
      <div className="text-sm font-medium">
        S{episode.seasonNumber}:E{episode.episodeNumber} — {episode.title}
      </div>
      <div className="mt-1 flex gap-2">
        <button
          onClick={onSkip}
          type="button"
          className="flex-1 rounded bg-white px-3 py-1.5 text-sm font-medium text-black transition-colors hover:bg-white/90"
        >
          Play Now
        </button>
        <button
          onClick={onCancel}
          type="button"
          className="rounded border border-white/30 px-3 py-1.5 text-sm transition-colors hover:bg-white/10"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
