import "@/styles/player-hud.css";
import { useEffect, useState } from "react";
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
import { useT } from "@/i18n";

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
  /** True while the browser decode engine plays the stream. */
  browserEngine?: boolean;
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
  browserEngine = false,
  onClose,
}: PlaybackInfoOverlayProps) {
  const t = useT();
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

  // Rebuilt on every render: the stats change every second anyway, and the
  // rows follow a language change through `t()`.
  const sections = buildPlaybackInfoSections({
    streamUrl,
    plan,
    currentSourceVersion,
    requestedVersion,
    runtimeStats,
    activeAudioTrack,
    qualityPreference,
    browserEngine,
  });

  return (
    <div
      // A labelled region, not a dialog: it is not modal, and a dialog would
      // switch off the player's shortcuts and the skip pill's focus.
      role="region"
      aria-label={t("web.player.info.heading")}
      className="hud-panel hud-panel-scroll absolute top-[max(5rem,calc(env(safe-area-inset-top)+4.5rem))] left-[max(1rem,env(safe-area-inset-left))] z-50 max-h-[max(10rem,calc(100%-14rem))] w-[min(20rem,calc(100vw-2rem))] sm:left-[max(2rem,env(safe-area-inset-left))] rtl:right-[max(1rem,env(safe-area-inset-right))] rtl:left-auto sm:rtl:right-[max(2rem,env(safe-area-inset-right))]"
    >
      <div className="hud-panel-header pe-2">
        <span>{t("web.player.info.heading")}</span>
        <button
          type="button"
          onClick={onClose}
          className="hud-icon-btn"
          aria-label={t("web.player.info.close")}
        >
          <X />
        </button>
      </div>

      <div className="px-4 pb-3">
        {sections.map((section) => (
          <section key={section.title} className="mt-3 first:mt-0">
            <h3 className="mb-1 text-[0.6875rem] font-bold tracking-[0.1em] text-[var(--hud-ash)] uppercase">
              {section.title}
            </h3>
            <dl>
              {section.rows.map((row, index) => (
                <div
                  key={`${row.label}-${index}`}
                  className="flex justify-between gap-4 py-0.5 text-[0.8125rem]"
                >
                  <dt className="shrink-0 text-[var(--hud-ash)]">{row.label}</dt>
                  <dd className="truncate text-end tabular-nums" title={row.value}>
                    {row.value}
                  </dd>
                </div>
              ))}
            </dl>
          </section>
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
