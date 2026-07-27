import { Link, useNavigate } from "react-router";
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

/**
 * Home-row teaser for currently airing Live TV programmes.
 * Hidden when the server has no enabled channels.
 */
export default function LiveTVOnNowRow() {
  const navigate = useNavigate();
  const channelsQuery = useLiveTVChannels();
  const channels = (channelsQuery.data ?? []).filter((ch) => ch.enabled).slice(0, 24);

  const guideWindow = (() => {
    const now = Date.now();
    return {
      start: new Date(now - 15 * 60 * 1000).toISOString(),
      end: new Date(now + 3 * 60 * 60 * 1000).toISOString(),
    };
  })();

  const guide = useLiveTVGuide(
    {
      channelIds: channels.map((ch) => ch.id),
      start: guideWindow.start,
      end: guideWindow.end,
    },
    channels.length > 0,
  );

  if (channelsQuery.isLoading || channels.length === 0) {
    return null;
  }

  const programs = guide.data?.programs ?? [];
  const now = new Date();
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
        skeletonAspect="aspect-video"
        onViewAll={() => navigate("/livetv")}
      >
        {cards.map(({ channel, nowProgram }) => {
          const progress = progressFraction(nowProgram.start, nowProgram.stop, now);
          return (
            <Link
              key={channel.id}
              to="/livetv?tab=guide"
              className="group block w-[200px] sm:w-[220px] lg:w-[240px]"
            >
              <div className="bg-muted relative aspect-video overflow-hidden rounded-lg">
                {channel.logo_url ? (
                  <img
                    src={channel.logo_url}
                    alt=""
                    className="absolute inset-0 m-auto max-h-[55%] max-w-[55%] object-contain opacity-80"
                  />
                ) : (
                  <div className="text-muted-foreground absolute inset-0 flex items-center justify-center">
                    <Radio className="h-8 w-8" aria-hidden />
                  </div>
                )}
                <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-transparent to-transparent" />
                <div className="absolute right-2 bottom-2 left-2">
                  <div className="mb-1 flex items-center gap-1.5">
                    <Badge variant="secondary">Live</Badge>
                    <span className="text-muted-foreground text-[11px] font-medium tabular-nums">
                      {channelDisplayNumber(channel)}
                    </span>
                  </div>
                  <div className="bg-background/40 h-1 overflow-hidden rounded-full">
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
