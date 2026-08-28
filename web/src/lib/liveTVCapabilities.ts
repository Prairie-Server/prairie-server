import type { LiveTVClientCapabilities } from "@/hooks/queries/useLiveTV";
import type { ClientCodecCapabilities } from "@/player/types";

/**
 * Audio codecs hls.js can transmux out of the MPEG-TS segments the live bridge
 * produces. Everything else has to be re-encoded server-side even when the
 * browser could decode it from a different container.
 */
const TS_TRANSMUXABLE_AUDIO = new Set(["aac", "mp3"]);

/**
 * Builds the capability payload for a Live TV tune.
 *
 * `MediaSource.isTypeSupported` answers "can this browser decode the codec",
 * which is not the same question as "can it receive the codec here". Chrome and
 * Edge report AC-3 and E-AC-3 as supported, but hls.js cannot lift either out of
 * MPEG-TS, so advertising them made the server copy the broadcast audio and
 * playback stalled after the first video frame with no sound. Narrowing the list
 * to what this delivery path can actually carry makes the server encode the rest
 * to AAC.
 *
 * Native players (Tizen AVPlay, ExoPlayer, AVPlayer, Roku) demux MPEG-TS
 * themselves and are unaffected — they report their own capabilities, or none at
 * all, and keep the cheap copy.
 */
export function buildLiveTVCapabilities(
  capabilities: ClientCodecCapabilities,
): LiveTVClientCapabilities {
  return {
    codecs_video: capabilities.codecs_video,
    codecs_audio: capabilities.codecs_audio.filter((codec) =>
      TS_TRANSMUXABLE_AUDIO.has(codec.trim().toLowerCase()),
    ),
    max_resolution: capabilities.max_resolution,
  };
}
