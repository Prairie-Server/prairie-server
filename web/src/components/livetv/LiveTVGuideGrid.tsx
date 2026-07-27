import type { LiveTVChannel, LiveTVProgram } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  buildGuideWindow,
  channelDisplayNumber,
  formatGuideTime,
  guideTimeTicks,
  layoutProgramsForChannel,
} from "@/lib/liveTVGuide";
import { cn } from "@/lib/utils";
import { Circle, Play } from "lucide-react";

const CHANNEL_COL_WIDTH = 200;
const ROW_HEIGHT = 88;

type LiveTVGuideGridProps = {
  channels: LiveTVChannel[];
  programs: LiveTVProgram[];
  selectedChannelId: string | null;
  now?: Date;
  onSelectChannel: (channelId: string) => void;
  onWatch: (channelId: string) => void;
  onRecord: (programId: string) => void;
  recordDisabled?: boolean;
  watchDisabled?: boolean;
};

export function LiveTVGuideGrid({
  channels,
  programs,
  selectedChannelId,
  now = new Date(),
  onSelectChannel,
  onWatch,
  onRecord,
  recordDisabled,
  watchDisabled,
}: LiveTVGuideGridProps) {
  const window = buildGuideWindow(now);
  const ticks = guideTimeTicks(window);
  const gridWidth = (window.endMs - window.startMs) * window.pxPerMs;
  const nowOffset = Math.min(
    gridWidth,
    Math.max(0, (now.getTime() - window.startMs) * window.pxPerMs),
  );

  return (
    <div className="border-border overflow-hidden rounded-xl border">
      <div className="max-h-[calc(100dvh-10rem)] overflow-auto">
        <div className="sticky top-0 z-20 flex border-b bg-[color-mix(in_srgb,var(--background)_92%,transparent)] backdrop-blur-sm">
          <div
            className="border-border text-muted-foreground sticky left-0 z-30 flex shrink-0 items-end border-r px-3 py-2.5 text-[10px] font-semibold tracking-[0.18em] uppercase"
            style={{ width: CHANNEL_COL_WIDTH }}
          >
            Channel
          </div>
          <div className="relative" style={{ width: gridWidth, minWidth: gridWidth }}>
            {ticks.map((tick) => (
              <div
                key={tick}
                className="border-border text-muted-foreground absolute top-0 bottom-0 border-l px-2.5 py-2.5 text-xs"
                style={{ left: (tick - window.startMs) * window.pxPerMs }}
              >
                {formatGuideTime(new Date(tick).toISOString())}
              </div>
            ))}
            <div className="h-10" />
          </div>
        </div>

        {channels.map((channel) => {
          const laid = layoutProgramsForChannel(programs, channel.id, window, now);
          const selected = selectedChannelId === channel.id;
          return (
            <div
              key={channel.id}
              className={cn(
                "border-border flex border-b last:border-b-0",
                selected && "bg-muted/40",
              )}
            >
              <button
                type="button"
                onClick={() => onSelectChannel(channel.id)}
                className="border-border hover:bg-muted/60 sticky left-0 z-10 flex shrink-0 flex-col justify-center gap-1 border-r bg-[color-mix(in_srgb,var(--background)_96%,transparent)] px-3.5 text-left"
                style={{ width: CHANNEL_COL_WIDTH, height: ROW_HEIGHT }}
              >
                <span className="flex items-center gap-2 text-sm font-medium">
                  <span className="text-muted-foreground tabular-nums">
                    {channelDisplayNumber(channel)}
                  </span>
                  <span className="truncate">{channel.callsign || channel.name}</span>
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {laid.find((p) => p.isNow)?.title ?? "No guide data"}
                </span>
              </button>
              <div
                className="relative"
                style={{ width: gridWidth, minWidth: gridWidth, height: ROW_HEIGHT }}
              >
                <div
                  className="bg-primary/70 absolute top-1.5 bottom-1.5 z-10 w-px"
                  style={{ left: nowOffset }}
                  aria-hidden
                />
                {laid.map((program) => (
                  <div
                    key={program.id}
                    className={cn(
                      "border-border absolute top-2 bottom-2 overflow-hidden rounded-md border px-2.5 py-1.5",
                      program.isNow
                        ? "bg-primary/15 border-primary/40"
                        : "bg-muted/50 hover:bg-muted",
                    )}
                    style={{ left: program.leftPx, width: program.widthPx }}
                    title={`${program.title} · ${formatGuideTime(program.start)} – ${formatGuideTime(program.stop)}`}
                  >
                    <div className="flex h-full items-center justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{program.title}</p>
                        <p className="text-muted-foreground truncate text-[11px]">
                          {formatGuideTime(program.start)} – {formatGuideTime(program.stop)}
                        </p>
                      </div>
                      <div className="flex shrink-0 gap-1">
                        {program.isNow ? (
                          <Button
                            size="sm"
                            variant="secondary"
                            className="h-8 px-2.5"
                            disabled={watchDisabled}
                            onClick={() => {
                              onSelectChannel(channel.id);
                              onWatch(channel.id);
                            }}
                          >
                            <Play />
                          </Button>
                        ) : null}
                        {program.canRecord ? (
                          <Button
                            size="sm"
                            variant="outline"
                            className="h-8 px-2.5"
                            disabled={recordDisabled}
                            onClick={() => onRecord(program.id)}
                            aria-label={`Record ${program.title}`}
                          >
                            <Circle />
                          </Button>
                        ) : null}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
