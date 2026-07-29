import { useEffect, useState } from "react";
import { usePlayerConfig } from "../context/PlayerConfigContext";
import { playerFetch } from "../player-fetch";

/** One rung of the server's transcode ladder. */
export interface QualityLadderRung {
  id: string;
  label: string;
  resolution: string;
  height: number;
  bitrate_kbps: number;
}

interface QualityLadderResponse {
  rungs: QualityLadderRung[];
  modes: string[];
  source_height?: number;
}

/**
 * Fallback ladder, used only when the server cannot be reached.
 *
 * Mirrors internal/playback/quality_ladder.go. Kept so a failed request degrades
 * to the previous hardcoded behaviour instead of leaving the quality menu empty,
 * which would strip a viewer's ability to escape a stalling stream. It is
 * deliberately the *only* copy of these numbers left in the web client — the
 * live values come from the server so its adaptive advice can never name a rung
 * the menu cannot select.
 */
const FALLBACK_LADDER: QualityLadderRung[] = [
  { id: "2160p", label: "4K", resolution: "2160p", height: 2160, bitrate_kbps: 20000 },
  { id: "1080p-high", label: "1080p High", resolution: "1080p", height: 1080, bitrate_kbps: 10000 },
  { id: "1080p", label: "1080p", resolution: "1080p", height: 1080, bitrate_kbps: 6000 },
  { id: "720p-high", label: "720p High", resolution: "720p", height: 720, bitrate_kbps: 4000 },
  { id: "720p", label: "720p", resolution: "720p", height: 720, bitrate_kbps: 2000 },
  { id: "480p", label: "480p", resolution: "480p", height: 480, bitrate_kbps: 1500 },
  { id: "420p", label: "420p", resolution: "420p", height: 420, bitrate_kbps: 720 },
];

/** Module-level cache: the ladder is server configuration, not per-session. */
let cachedLadder: QualityLadderRung[] | null = null;
let inFlight: Promise<QualityLadderRung[]> | null = null;

async function loadLadder(
  config: ReturnType<typeof usePlayerConfig>,
): Promise<QualityLadderRung[]> {
  if (cachedLadder) return cachedLadder;
  inFlight ??= playerFetch<QualityLadderResponse>(config, "/playback/quality-ladder", {
    method: "GET",
  })
    .then((resp) => {
      // An empty or malformed ladder is treated as a failure: an empty quality
      // menu is worse than a stale one.
      const rungs = resp?.rungs?.filter((rung) => rung?.id && rung.height > 0) ?? [];
      cachedLadder = rungs.length > 0 ? rungs : FALLBACK_LADDER;
      return cachedLadder;
    })
    .catch(() => {
      // Do not cache a failure — a transient error should not pin the fallback
      // for the rest of the page's lifetime.
      return FALLBACK_LADDER;
    })
    .finally(() => {
      inFlight = null;
    });
  return inFlight;
}

/**
 * The server's transcode ladder, highest rung first.
 *
 * Returns the fallback synchronously on first render so the quality menu is
 * never empty, then re-renders with the server's ladder once loaded.
 */
export function useQualityLadder(): QualityLadderRung[] {
  const config = usePlayerConfig();
  const [ladder, setLadder] = useState<QualityLadderRung[]>(cachedLadder ?? FALLBACK_LADDER);

  useEffect(() => {
    if (cachedLadder) {
      setLadder(cachedLadder);
      return;
    }
    let active = true;
    void loadLadder(config).then((loaded) => {
      if (active) setLadder(loaded);
    });
    return () => {
      active = false;
    };
  }, [config]);

  return ladder;
}

/** Test seam: clears the module-level cache between cases. */
export function __resetQualityLadderCache(): void {
  cachedLadder = null;
  inFlight = null;
}
