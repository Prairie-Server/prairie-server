import { useEffect, useRef, useState } from "react";
import mpegts from "mpegts.js";
import { Loader2 } from "lucide-react";
import { getAccessToken, getProfileId, getProfileToken } from "@/api/client";
import { cn } from "@/lib/utils";

type LiveTVPlayerProps = {
  streamUrl: string;
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

/**
 * Plays a Live TV MPEG-TS session proxy with mpegts.js.
 * Auth headers are attached via mpegts config so RequireProfile succeeds.
 */
export function LiveTVPlayer({ streamUrl, title, className }: LiveTVPlayerProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const playerRef = useRef<ReturnType<typeof mpegts.createPlayer> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(true);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !streamUrl) return;

    setError(null);
    setStarting(true);

    if (!mpegts.isSupported() || !mpegts.getFeatureList().mseLivePlayback) {
      setError("Live MPEG-TS playback is not supported in this browser.");
      setStarting(false);
      return;
    }

    const player = mpegts.createPlayer(
      {
        type: "mpegts",
        isLive: true,
        url: resolveStreamUrl(streamUrl),
      },
      {
        enableStashBuffer: false,
        liveBufferLatencyChasing: true,
        liveSync: true,
        headers: authHeaders(),
      },
    );
    playerRef.current = player;
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
      if (playerRef.current === player) {
        playerRef.current = null;
      }
    };
  }, [streamUrl]);

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
