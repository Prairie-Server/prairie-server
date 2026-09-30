import { useEffect, useMemo, useState } from "react";
import { X } from "lucide-react";
import type HlsType from "hls.js";
import { buildPlaybackInfoSections, type RuntimePlaybackStats } from "../playback-info";
import {
  formatPlaybackEventTime,
  type PlaybackEventEntry,
  type PlaybackEventLog,
} from "../playback-events";
import type { PlanV3 } from "../protocol-v3";
import type { PlayerAudioTrack, PlayerFileVersion } from "../types";

interface PlaybackInfoOverlayProps {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  containerRef: React.RefObject<HTMLDivElement | null>;
  streamUrl: string;
  /** The route the server chose, and the only description of what is on the wire. */
  plan: PlanV3;
  currentSourceVersion?: PlayerFileVersion;
  requestedVersion?: PlayerFileVersion;
  activeAudioTrack?: PlayerAudioTrack;
  qualityPreference?: string;
  hlsRef?: React.RefObject<HlsType | null>;
  /** Recent player events, recorded whether or not the overlay is open. */
  eventLog?: PlaybackEventLog;
  onClose: () => void;
}

export function PlaybackInfoOverlay({
  videoRef,
  containerRef,
  streamUrl,
  plan,
  currentSourceVersion,
  requestedVersion,
  activeAudioTrack,
  qualityPreference,
  hlsRef,
  eventLog,
  onClose,
}: PlaybackInfoOverlayProps) {
  const [runtimeStats, setRuntimeStats] = useState<RuntimePlaybackStats>({});
  const [events, setEvents] = useState<PlaybackEventEntry[]>(() => eventLog?.snapshot() ?? []);

  // Poll runtime stats every second.
  useEffect(() => {
    function collect() {
      const video = videoRef.current;
      const container = containerRef.current;
      if (!video) return;

      const quality = (
        video as HTMLVideoElement & {
          getVideoPlaybackQuality?: () => VideoPlaybackQuality;
        }
      ).getVideoPlaybackQuality?.();

      const bandwidth = hlsRef?.current?.bandwidthEstimate;
      setRuntimeStats({
        playerWidth: container?.clientWidth,
        playerHeight: container?.clientHeight,
        videoWidth: video.videoWidth || undefined,
        videoHeight: video.videoHeight || undefined,
        droppedFrames: quality?.droppedVideoFrames ?? null,
        corruptedFrames: quality?.corruptedVideoFrames ?? null,
        bufferAheadSeconds: bufferAhead(video),
        bandwidthEstimateKbps:
          Number.isFinite(bandwidth) && bandwidth != null && bandwidth > 0
            ? Math.round(bandwidth / 1000)
            : undefined,
      });
      if (eventLog) setEvents(eventLog.snapshot());
    }

    collect();
    const id = setInterval(collect, 1000);
    return () => clearInterval(id);
  }, [videoRef, containerRef, hlsRef, eventLog]);

  const sections = useMemo(
    () =>
      buildPlaybackInfoSections({
        streamUrl,
        plan,
        currentSourceVersion,
        requestedVersion,
        runtimeStats,
        activeAudioTrack,
        qualityPreference,
      }),
    [
      streamUrl,
      plan,
      currentSourceVersion,
      requestedVersion,
      runtimeStats,
      activeAudioTrack,
      qualityPreference,
    ],
  );

  return (
    <div className="absolute top-12 right-3 left-3 z-50 max-h-[calc(100%-6rem)] w-auto overflow-y-auto rounded-lg bg-black/85 text-sm text-white shadow-lg backdrop-blur-sm sm:right-auto sm:left-4 sm:w-96">
      <div className="flex items-center justify-between px-4 pt-3 pb-2">
        <span className="font-medium text-white/90">Stats for nerds</span>
        <button
          type="button"
          onClick={onClose}
          className="flex h-6 w-6 items-center justify-center rounded hover:bg-white/10"
          aria-label="Close stats for nerds"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="px-4 pb-3">
        {sections.map((section) => (
          <div key={section.title} className="mt-3 first:mt-0">
            <div className="mb-1 text-xs font-semibold tracking-wider text-white/50 uppercase">
              {section.title}
            </div>
            {section.rows.map((row, index) => (
              <div key={`${row.label}-${index}`} className="flex justify-between gap-4 py-0.5">
                <span className="shrink-0 text-white/60">{row.label}</span>
                <span className="truncate text-right text-white/90" title={row.value}>
                  {row.value}
                </span>
              </div>
            ))}
          </div>
        ))}
        {eventLog && (
          <div className="mt-3">
            <div className="mb-1 text-xs font-semibold tracking-wider text-white/50 uppercase">
              Recent events
            </div>
            {events.length === 0 ? (
              <div className="py-0.5 text-white/60">None yet</div>
            ) : (
              <ol aria-label="Recent player events">
                {[...events].reverse().map((entry, index) => (
                  <li key={`${entry.at}-${index}`} className="flex gap-3 py-0.5">
                    <span className="shrink-0 font-mono text-xs leading-5 text-white/50">
                      {formatPlaybackEventTime(entry.at)}
                    </span>
                    <span className="break-words text-white/90">{entry.message}</span>
                  </li>
                ))}
              </ol>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function bufferAhead(video: HTMLVideoElement): number | undefined {
  const { buffered, currentTime } = video;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) <= currentTime + 0.25 && currentTime <= buffered.end(index)) {
      return Math.max(0, buffered.end(index) - currentTime);
    }
  }
  return buffered.length ? 0 : undefined;
}
