/**
 * A short, timestamped history of what the player just did, for the stats
 * overlay. It records continuously so that opening the overlay after a stall
 * or error still shows how playback got there.
 */
export interface PlaybackEventEntry {
  /** Milliseconds since the epoch. */
  at: number;
  message: string;
}

export const PLAYBACK_EVENT_LOG_CAPACITY = 8;

export class PlaybackEventLog {
  private readonly capacity: number;
  private entries: PlaybackEventEntry[] = [];

  constructor(capacity = PLAYBACK_EVENT_LOG_CAPACITY) {
    this.capacity = Math.max(1, capacity);
  }

  add(message: string, at = Date.now()): void {
    const trimmed = message.trim();
    if (!trimmed) return;
    const last = this.entries[this.entries.length - 1];
    // A stalling stream fires the same event repeatedly; one line per burst
    // keeps the older, more telling entries on screen.
    if (last && last.message === trimmed && at - last.at < 1000) {
      this.entries[this.entries.length - 1] = { at, message: trimmed };
      return;
    }
    this.entries = [...this.entries, { at, message: trimmed }].slice(-this.capacity);
  }

  /** Oldest first. The returned array is a snapshot. */
  snapshot(): PlaybackEventEntry[] {
    return this.entries;
  }

  clear(): void {
    this.entries = [];
  }
}

/** Wall-clock time with seconds, e.g. "14:03:27". */
export function formatPlaybackEventTime(at: number): string {
  const date = new Date(at);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

const MEDIA_ERROR_NAMES: Record<number, string> = {
  1: "aborted",
  2: "network",
  3: "decode",
  4: "source not supported",
};

/** Describes an HTMLMediaElement event for the log, or null to skip it. */
export function describeMediaEvent(type: string, video: HTMLVideoElement): string | null {
  const at = Number.isFinite(video.currentTime) ? ` @ ${video.currentTime.toFixed(1)}s` : "";
  switch (type) {
    case "loadedmetadata":
      return video.videoWidth && video.videoHeight
        ? `metadata ${video.videoWidth}x${video.videoHeight}`
        : "metadata loaded";
    case "playing":
      return `playing${at}`;
    case "waiting":
      return `buffering${at}`;
    case "stalled":
      return `stalled${at}`;
    case "seeking":
      return `seeking to ${video.currentTime.toFixed(1)}s`;
    case "seeked":
      return `seeked${at}`;
    case "ended":
      return "ended";
    case "error": {
      const error = video.error;
      if (!error) return "media error";
      const name = MEDIA_ERROR_NAMES[error.code] ?? `code ${error.code}`;
      return error.message ? `media error (${name}): ${error.message}` : `media error (${name})`;
    }
    default:
      return null;
  }
}

export const LOGGED_MEDIA_EVENTS = [
  "loadedmetadata",
  "playing",
  "waiting",
  "stalled",
  "seeking",
  "seeked",
  "ended",
  "error",
] as const;
