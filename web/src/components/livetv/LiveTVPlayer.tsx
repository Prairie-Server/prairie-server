import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import mpegts from "mpegts.js";
import { Loader2 } from "lucide-react";
import { getAccessToken, getProfileId, getProfileToken } from "@/api/client";
import { cn } from "@/lib/utils";

type LiveTVPlayerProps = {
  streamUrl: string;
  /** Defaults to mpegts when omitted. */
  transport?: "mpegts" | "hls";
  title?: string;
  className?: string;
};

function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = {};
  const token = getAccessToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const profileId = getProfileId();
  if (profileId) headers["X-Profile-Id"] = profileId;
  const profileToken = getProfileToken();
  if (profileToken) headers["X-Profile-Token"] = profileToken;
  return headers;
}

function resolveStreamUrl(streamUrl: string): string {
  if (streamUrl.startsWith("http://") || streamUrl.startsWith("https://")) {
    return streamUrl;
  }
  if (typeof window === "undefined") return streamUrl;
  return new URL(streamUrl, window.location.origin).toString();
}

function withMediaAuthQuery(url: string): string {
  const params = new URLSearchParams();
  const token = getAccessToken();
  if (token) params.set("token", token);
  const profileId = getProfileId();
  if (profileId) params.set("profile_id", profileId);
  const encoded = params.toString();
  if (!encoded) return url;
  const separator = url.includes("?") ? "&" : "?";
  return `${url}${separator}${encoded}`;
}

/**
 * Plays a Live TV stream. Uses hls.js for remuxed HLS (`transport=hls`) and
 * mpegts.js for the MPEG-TS session proxy.
 */
export function LiveTVPlayer({
  streamUrl,
  transport = "mpegts",
  title,
  className,
}: LiveTVPlayerProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const mpegtsRef = useRef<ReturnType<typeof mpegts.createPlayer> | null>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(true);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !streamUrl) return;

    setError(null);
    setStarting(true);

    const url = resolveStreamUrl(streamUrl);
    const isHLS = transport === "hls" || url.includes(".m3u8");

    if (isHLS) {
      const playUrl = withMediaAuthQuery(url);
      if (!Hls.isSupported()) {
        // Safari can play HLS natively but cannot attach auth headers; require
        // hls.js (MSE) so X-Profile-Id reaches RequireProfile.
        setError("HLS playback requires Media Source Extensions in this browser.");
        setStarting(false);
        return;
      }
      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: true,
        xhrSetup: (xhr) => {
          const headers = authHeaders();
          for (const [key, value] of Object.entries(headers)) {
            xhr.setRequestHeader(key, value);
          }
        },
      });
      hlsRef.current = hls;
      hls.loadSource(playUrl);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        void video.play().then(
          () => setStarting(false),
          () => setStarting(false),
        );
      });
      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal) {
          setError(data.details || "HLS playback error");
          setStarting(false);
        }
      });
      return () => {
        hls.destroy();
        if (hlsRef.current === hls) hlsRef.current = null;
      };
    }

    if (!mpegts.isSupported() || !mpegts.getFeatureList().mseLivePlayback) {
      setError("Live MPEG-TS playback is not supported in this browser.");
      setStarting(false);
      return;
    }

    const player = mpegts.createPlayer(
      {
        type: "mpegts",
        isLive: true,
        url,
      },
      {
        enableStashBuffer: false,
        liveBufferLatencyChasing: true,
        liveSync: true,
        headers: authHeaders(),
      },
    );
    mpegtsRef.current = player;
    player.attachMediaElement(video);
    player.on(mpegts.Events.ERROR, (_type: string, _detail: string, info: unknown) => {
      const message =
        typeof info === "object" && info && "msg" in info
          ? String((info as { msg?: string }).msg ?? "Playback error")
          : "Playback error";
      setError(message);
      setStarting(false);
    });
    player.load();
    const playResult = player.play();
    if (playResult && typeof playResult.then === "function") {
      void playResult.then(
        () => setStarting(false),
        () => setStarting(false),
      );
    } else {
      setStarting(false);
    }

    return () => {
      try {
        player.pause();
        player.unload();
        player.detachMediaElement();
        player.destroy();
      } catch {
        // Best-effort teardown when the session URL changes.
      }
      if (mpegtsRef.current === player) mpegtsRef.current = null;
    };
  }, [streamUrl, transport]);

  return (
    <div className={cn("relative overflow-hidden bg-black", className)}>
      <video
        ref={videoRef}
        className="h-full w-full object-contain"
        controls
        playsInline
        autoPlay
        aria-label={title ? `Live: ${title}` : "Live TV"}
      />
      {starting ? (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-black/40">
          <Loader2 className="h-8 w-8 animate-spin text-white" aria-hidden />
        </div>
      ) : null}
      {error ? (
        <div className="absolute inset-x-0 bottom-0 bg-black/80 px-3 py-2 text-sm text-white">
          {error}
        </div>
      ) : null}
    </div>
  );
}
