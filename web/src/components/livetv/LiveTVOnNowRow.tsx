import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { Play, Radio } from "lucide-react";
import MediaCarousel from "@/components/MediaCarousel";
import { Badge } from "@/components/ui/badge";
import { useLiveTVChannels, useLiveTVGuide } from "@/hooks/queries/useLiveTV";
import {
  channelDisplayNumber,
  channelLabel,
  formatGuideTime,
  pickNowNext,
  progressFraction,
} from "@/lib/liveTVGuide";
import { buildLiveWatchHref } from "@/lib/liveTVWatch";

/**
 * Home-row teaser for currently airing Live TV programmes.
 * Hidden when the server has no enabled channels.
 *
 * Guide window bounds are pinned to a minute-bucketed clock so the React Query
 * key stays stable across renders (a fresh Date.now() ISO string every paint
 * was hammering GET /livetv/guide).
 */
export default function LiveTVOnNowRow() {
  const channelsQuery = useLiveTVChannels();
  const channels = useMemo(
    () => (channelsQuery.data ?? []).filter((ch) => ch.enabled).slice(0, 24),
    [channelsQuery.data],
  );
  const channelIds = useMemo(() => channels.map((ch) => ch.id), [channels]);

  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNowMs(Date.now()), 60_000);
    return () => window.clearInterval(id);
  }, []);
  const now = useMemo(() => new Date(nowMs), [nowMs]);

  const guideWindow = useMemo(
    () => ({
      start: new Date(nowMs - 15 * 60 * 1000).toISOString(),
      end: new Date(nowMs + 3 * 60 * 60 * 1000).toISOString(),
    }),
    [nowMs],
  );

  const guide = useLiveTVGuide(
    {
      channelIds,
      start: guideWindow.start,
      end: guideWindow.end,
    },
    channelIds.length > 0,
  );

  if (channelsQuery.isLoading || channelsQuery.isError || channels.length === 0) {
    return null;
  }

  const programs = guide.data?.programs ?? [];
  const cards = channels
    .map((channel) => {
      const slot = pickNowNext(programs, channel.id, now);
      if (!slot.now) return null;
      return { channel, nowProgram: slot.now };
    })
    .filter((row): row is NonNullable<typeof row> => row != null)
    .slice(0, 16);

  if (cards.length === 0 && !guide.isLoading) {
    return null;
  }

  return (
    <div className="section-fade-in">
      <MediaCarousel
        title="On now"
        titleHref="/livetv"
        loading={guide.isLoading && cards.length === 0}
        skeletonAspect="aspect-[2/3]"
      >
        {cards.map(({ channel, nowProgram }) => {
          const progress = progressFraction(nowProgram.start, nowProgram.stop, now);
          const posterUrl = nowProgram.image_url?.trim() || "";
          const logoUrl = channel.logo_url?.trim() || "";
          return (
            <Link
              key={channel.id}
              to={buildLiveWatchHref(channel.id, "/")}
              className="group block w-[130px] sm:w-[150px] lg:w-[178px]"
            >
              <div className="bg-muted relative aspect-[2/3] overflow-hidden rounded-lg">
                {posterUrl ? (
                  <img
                    src={posterUrl}
                    alt=""
                    className="absolute inset-0 h-full w-full object-cover"
                    loading="lazy"
                  />
                ) : logoUrl ? (
                  <img
                    src={logoUrl}
                    alt=""
                    className="absolute inset-0 m-auto max-h-[55%] max-w-[55%] object-contain opacity-80"
                    loading="lazy"
                  />
                ) : (
                  <div className="text-muted-foreground absolute inset-0 flex items-center justify-center">
                    <Radio className="h-8 w-8" aria-hidden />
                  </div>
                )}
                <div className="absolute inset-0 bg-gradient-to-t from-black/75 via-black/10 to-transparent" />
                <div className="absolute right-2 bottom-2 left-2">
                  <div className="mb-1 flex items-center gap-1.5">
                    <Badge variant="secondary">Live</Badge>
                    <span className="text-[11px] font-medium text-white/80 tabular-nums">
                      {channelDisplayNumber(channel)}
                    </span>
                  </div>
                  <div className="h-1 overflow-hidden rounded-full bg-white/25">
                    <div className="bg-primary h-full" style={{ width: `${progress * 100}%` }} />
                  </div>
                </div>
                <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity group-hover:opacity-100">
                  <span className="bg-background/80 text-foreground rounded-full p-2">
                    <Play className="h-5 w-5" fill="currentColor" />
                  </span>
                </div>
              </div>
              <p className="mt-2 truncate text-sm font-medium">{nowProgram.title}</p>
              <p className="text-muted-foreground truncate text-xs">
                {channelLabel(channel)} · until {formatGuideTime(nowProgram.stop)}
              </p>
            </Link>
          );
        })}
      </MediaCarousel>
    </div>
  );
}
