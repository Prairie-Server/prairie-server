import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { ArrowLeft, Pause, Play, Radio, Square } from "lucide-react";
import { LiveTVPlayer } from "@/components/livetv/LiveTVPlayer";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  useLiveTVChannels,
  useLiveTVGuide,
  useLiveTVSessionHeartbeat,
  useReleaseLiveTVSession,
  useStartLiveTVSession,
} from "@/hooks/queries/useLiveTV";
import { releaseLiveTVSessionOnUnload } from "@/lib/liveTVWatch";
import { buildLiveTVCapabilities } from "@/lib/liveTVCapabilities";
import { channelLabel, pickNowNext } from "@/lib/liveTVGuide";
import { CircleButton } from "@/player/components/CircleButton";
import { useCodecDetection } from "@/player/hooks/useCodecDetection";

/**
 * Fullscreen Live TV watch route — same shell as `/watch/:id` (fixed overlay,
 * player chrome, exit back to the prior page). Starts the channel session on
 * mount so Home "On now" and guide Play land directly in playback.
 */
export default function LiveWatchRoute() {
  const { channelId: rawChannelId } = useParams<{ channelId: string }>();
  const channelId = rawChannelId ? decodeURIComponent(rawChannelId) : "";
  const [searchParams] = useSearchParams();
  const returnHref = searchParams.get("return") || "/livetv";
  const navigate = useNavigate();

  const channelsQuery = useLiveTVChannels();
  const channels = useMemo(
    () => (channelsQuery.data ?? []).filter((ch) => ch.enabled),
    [channelsQuery.data],
  );
  const channel = channels.find((ch) => ch.id === channelId) ?? null;

  const [nowMs] = useState(() => Date.now());
  const guide = useLiveTVGuide(
    {
      channelIds: channelId ? [channelId] : [],
      start: new Date(nowMs - 15 * 60 * 1000).toISOString(),
      end: new Date(nowMs + 2 * 60 * 60 * 1000).toISOString(),
    },
    Boolean(channelId),
  );
  const nowProgram = useMemo(() => {
    if (!channelId) return null;
    return pickNowNext(guide.data?.programs ?? [], channelId, new Date(nowMs))
      .now;
  }, [channelId, guide.data?.programs, nowMs]);

  const title = channel
    ? nowProgram
      ? `${channelLabel(channel)} · ${nowProgram.title}`
      : channelLabel(channel)
    : "Live TV";
  useDocumentTitle(title);

  const startSession = useStartLiveTVSession();
  const releaseSession = useReleaseLiveTVSession();
  const { capabilities } = useCodecDetection();
  const sessionIdRef = useRef<string | null>(null);
  const releasedRef = useRef(false);
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [streamURL, setStreamURL] = useState<string | null>(null);
  const [transport, setTransport] = useState<"mpegts" | "hls">("hls");
  const [tuneError, setTuneError] = useState<string | null>(null);
  const [tuning, setTuning] = useState(true);
  const [playing, setPlaying] = useState(true);
  const [playerError, setPlayerError] = useState<string | null>(null);
  const [playerStarting, setPlayerStarting] = useState(true);
  const [controlsVisible, setControlsVisible] = useState(true);
  const hideTimerRef = useRef<number | null>(null);
  const videoHostRef = useRef<HTMLDivElement | null>(null);

  const exit = useCallback(async () => {
    if (releasedRef.current) {
      void navigate(returnHref);
      return;
    }
    releasedRef.current = true;
    const sid = sessionIdRef.current;
    sessionIdRef.current = null;
    setActiveSessionId(null);
    if (sid) {
      try {
        await releaseSession.mutateAsync(sid);
      } catch {
        // Best-effort release; still leave the player.
      }
    }
    void navigate(returnHref);
  }, [navigate, releaseSession, returnHref]);

  useEffect(() => {
    if (!channelId) return;
    let cancelled = false;
    releasedRef.current = false;
    setTuning(true);
    setTuneError(null);
    setStreamURL(null);
    setPlayerError(null);
    setPlayerStarting(true);

    void (async () => {
      try {
        const session = await startSession.mutateAsync({
          channelId,
          // Browsers cannot decode the MPEG-2 / AC-3 an OTA tuner emits; the
          // server re-encodes whatever is missing from this list.
          capabilities: buildLiveTVCapabilities(capabilities),
        });
        if (cancelled) {
          await releaseSession
            .mutateAsync(session.session_id)
            .catch(() => undefined);
          return;
        }
        sessionIdRef.current = session.session_id;
        setActiveSessionId(session.session_id);
        const nextUrl = session.hls_url || session.stream_url || null;
        setTransport(
          session.transport === "hls" || Boolean(session.hls_url)
            ? "hls"
            : "mpegts",
        );
        setStreamURL(nextUrl);
        if (!nextUrl) {
          setTuneError(
            "Live TV session started but no stream URL was returned",
          );
        }
      } catch (err) {
        if (!cancelled) {
          setTuneError(
            err instanceof Error ? err.message : "Could not start Live TV",
          );
        }
      } finally {
        if (!cancelled) setTuning(false);
      }
    })();

    return () => {
      cancelled = true;
      const sid = sessionIdRef.current;
      if (sid && !releasedRef.current) {
        releasedRef.current = true;
        sessionIdRef.current = null;
        void releaseSession.mutateAsync(sid).catch(() => undefined);
      }
    };
    // Start once per channel deep-link; hooks are stable enough for this mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId]);

  useLiveTVSessionHeartbeat(
    activeSessionId,
    Boolean(activeSessionId) && !tuneError,
  );

  // Closing the tab never runs React cleanup, so free the tuner with a beacon.
  useEffect(() => {
    function onPageHide() {
      const sid = sessionIdRef.current;
      if (!sid || releasedRef.current) return;
      releasedRef.current = true;
      sessionIdRef.current = null;
      releaseLiveTVSessionOnUnload(sid);
    }
    window.addEventListener("pagehide", onPageHide);
    return () => window.removeEventListener("pagehide", onPageHide);
  }, []);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        void exit();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [exit]);

  const bumpControls = useCallback(() => {
    setControlsVisible(true);
    if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
    hideTimerRef.current = window.setTimeout(
      () => setControlsVisible(false),
      3500,
    );
  }, []);

  useEffect(() => {
    bumpControls();
    return () => {
      if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
    };
  }, [bumpControls]);

  const togglePlay = useCallback(() => {
    const video = videoHostRef.current?.querySelector("video");
    if (!video) return;
    if (video.paused) {
      void video.play().then(
        () => setPlaying(true),
        () => setPlaying(false),
      );
    } else {
      video.pause();
      setPlaying(false);
    }
    bumpControls();
  }, [bumpControls]);

  const showSpinner =
    tuning || (Boolean(streamURL) && playerStarting && !playerError);
  const fatalError = tuneError || playerError;

  return (
    <div
      className="bg-background fixed inset-0 z-50 flex flex-col"
      onMouseMove={bumpControls}
      onClick={bumpControls}
      role="presentation"
    >
      <div ref={videoHostRef} className="relative min-h-0 flex-1 bg-black">
        {streamURL && !tuneError ? (
          <LiveTVPlayer
            streamUrl={streamURL}
            transport={transport}
            title={title}
            hideNativeControls
            className="absolute inset-0 h-full w-full"
            onPlayingChange={setPlaying}
            onErrorChange={setPlayerError}
            onStartingChange={setPlayerStarting}
          />
        ) : null}

        {showSpinner ? (
          <div className="pointer-events-none absolute inset-0 z-20 flex items-center justify-center bg-black/50">
            <div className="surface-panel-subtle flex min-w-[240px] flex-col items-center gap-3 rounded-[1.8rem] px-8 py-7 text-center">
              <div className="h-8 w-8 animate-spin rounded-full border-2 border-white/20 border-t-white" />
              <div className="space-y-1">
                <p className="text-sm font-medium text-white">Tuning…</p>
                <p className="text-xs text-white/55">
                  {channel ? channelLabel(channel) : "Starting live stream"}
                </p>
              </div>
            </div>
          </div>
        ) : null}

        {fatalError ? (
          <div className="absolute inset-0 z-30 flex items-center justify-center bg-black/80 px-6">
            <div className="surface-panel-subtle flex max-w-md flex-col items-center gap-4 rounded-[1.8rem] px-8 py-8 text-center">
              <div className="space-y-2">
                <p className="text-base font-semibold text-white">
                  Playback unavailable
                </p>
                <p className="text-sm text-white/60">{fatalError}</p>
              </div>
              <Button
                type="button"
                variant="secondary"
                onClick={() => void exit()}
              >
                Go Back
              </Button>
            </div>
          </div>
        ) : null}

        <div
          className={`pointer-events-none absolute inset-0 z-10 bg-gradient-to-t from-black/80 via-transparent to-black/50 transition-opacity duration-300 ${
            controlsVisible || fatalError || showSpinner
              ? "opacity-100"
              : "opacity-0"
          }`}
        />

        <div
          className={`absolute inset-x-0 top-0 z-20 flex items-start justify-between gap-4 p-4 transition-opacity duration-300 sm:p-6 ${
            controlsVisible || fatalError || showSpinner
              ? "pointer-events-auto opacity-100"
              : "pointer-events-none opacity-0"
          }`}
        >
          <div className="flex min-w-0 items-start gap-3">
            <Button
              type="button"
              size="icon"
              variant="ghost"
              className="text-white hover:bg-white/10 hover:text-white"
              onClick={() => void exit()}
              aria-label="Back"
            >
              <ArrowLeft />
            </Button>
            <div className="min-w-0 pt-1">
              <div className="mb-1 flex flex-wrap items-center gap-2">
                <Badge className="bg-red-600 text-white hover:bg-red-600">
                  <Radio className="mr-1 h-3 w-3" aria-hidden />
                  Live
                </Badge>
                {channel ? (
                  <span className="text-xs font-medium tracking-wide text-white/70 uppercase">
                    {channelLabel(channel)}
                  </span>
                ) : null}
              </div>
              <h1 className="truncate text-lg font-semibold text-white sm:text-xl">
                {nowProgram?.title || title}
              </h1>
              {nowProgram ? (
                <p className="mt-0.5 truncate text-sm text-white/60">
                  {channel ? channelLabel(channel) : null}
                </p>
              ) : null}
            </div>
          </div>
        </div>

        <div
          className={`absolute inset-x-0 bottom-0 z-20 flex items-center justify-center gap-4 p-6 transition-opacity duration-300 ${
            controlsVisible && !fatalError
              ? "pointer-events-auto opacity-100"
              : "pointer-events-none opacity-0"
          }`}
        >
          <CircleButton
            size="md"
            variant="primary"
            ariaLabel={playing ? "Pause" : "Play"}
            onClick={togglePlay}
            disabled={!streamURL || Boolean(fatalError)}
          >
            {playing ? (
              <Pause className="h-5 w-5" fill="currentColor" />
            ) : (
              <Play className="h-5 w-5" fill="currentColor" />
            )}
          </CircleButton>
          <CircleButton
            size="sm"
            variant="secondary"
            ariaLabel="Stop"
            onClick={() => void exit()}
          >
            <Square className="h-4 w-4" fill="currentColor" />
          </CircleButton>
        </div>
      </div>
    </div>
  );
}
