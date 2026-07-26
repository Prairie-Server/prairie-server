import { useEffect, useMemo, useState } from "react";
import { CircleDot, Radio, Square } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  useCancelLiveTVRecording,
  useLiveTVChannels,
  useLiveTVGuide,
  useLiveTVRecordings,
  useReleaseLiveTVSession,
  useScheduleLiveTVRecording,
  useStartLiveTVSession,
} from "@/hooks/queries/useLiveTV";
import {
  channelDisplayNumber,
  channelLabel,
  formatGuideTime,
  pickNowNext,
} from "@/lib/liveTVGuide";
import { cn } from "@/lib/utils";

export default function LiveTV() {
  useDocumentTitle("Live TV");
  const channelsQuery = useLiveTVChannels();
  const channels = useMemo(
    () => (channelsQuery.data ?? []).filter((ch) => ch.enabled),
    [channelsQuery.data],
  );
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selected = channels.find((ch) => ch.id === selectedId) ?? channels[0] ?? null;

  useEffect(() => {
    if (!selectedId && channels[0]) {
      setSelectedId(channels[0].id);
    }
  }, [channels, selectedId]);

  // Capture wall-clock once on mount; recomputing Date.now() during render trips
  // react-hooks/purity and would also churn the guide query key every render.
  const [guideWindow] = useState(() => {
    const now = Date.now();
    return {
      start: new Date(now - 30 * 60 * 1000).toISOString(),
      end: new Date(now + 6 * 60 * 60 * 1000).toISOString(),
    };
  });

  const guide = useLiveTVGuide(
    {
      channelIds: channels.map((ch) => ch.id),
      start: guideWindow.start,
      end: guideWindow.end,
    },
    channels.length > 0,
  );
  const programs = guide.data?.programs ?? [];
  const recordings = useLiveTVRecordings();
  const startSession = useStartLiveTVSession();
  const releaseSession = useReleaseLiveTVSession();
  const scheduleRecording = useScheduleLiveTVRecording();
  const cancelRecording = useCancelLiveTVRecording();
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [streamURL, setStreamURL] = useState<string | null>(null);

  const selectedGuide = selected
    ? pickNowNext(programs, selected.id)
    : { now: null, next: null };

  async function onWatch() {
    if (!selected) return;
    const session = await startSession.mutateAsync(selected.id);
    setActiveSessionId(session.session_id);
    setStreamURL(session.hls_url || session.stream_url || null);
    if (session.note) {
      toast.message(session.note);
    } else {
      toast.success(`Watching ${channelLabel(selected)}`);
    }
  }

  async function onStop() {
    if (!activeSessionId) {
      setStreamURL(null);
      return;
    }
    await releaseSession.mutateAsync(activeSessionId);
    setActiveSessionId(null);
    setStreamURL(null);
    toast.success("Live TV session released");
  }

  function onRecordNow() {
    if (!selected || !selectedGuide.now) return;
    scheduleRecording.mutate({ program_id: selectedGuide.now.id });
  }

  function onRecordNext() {
    if (!selected || !selectedGuide.next) return;
    scheduleRecording.mutate({ program_id: selectedGuide.next.id });
  }

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-4 py-8 sm:px-6">
      <header className="space-y-2">
        <div className="flex flex-wrap items-center gap-3">
          <Radio className="text-primary h-7 w-7" aria-hidden />
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Live TV</h1>
          <Badge variant="secondary">{channels.length} channels</Badge>
        </div>
        <p className="text-muted-foreground max-w-2xl text-sm leading-6">
          Browse your OTA lineup, see what&apos;s on now, and start a watch or recording session.
        </p>
      </header>

      {channelsQuery.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading channels…</p>
      ) : channels.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No Live TV channels yet. An admin can add an HDHomeRun tuner under Admin → Live TV.
        </p>
      ) : (
        <div className="grid gap-8 lg:grid-cols-[minmax(0,16rem)_minmax(0,1fr)]">
          <aside className="min-w-0">
            <h2 className="text-muted-foreground mb-3 text-[10px] font-semibold tracking-[0.22em] uppercase">
              Channels
            </h2>
            <ul className="divide-border max-h-[70vh] divide-y overflow-y-auto border-y">
              {channels.map((channel) => {
                const active = selected?.id === channel.id;
                const guideSlot = pickNowNext(programs, channel.id);
                return (
                  <li key={channel.id}>
                    <button
                      type="button"
                      onClick={() => setSelectedId(channel.id)}
                      className={cn(
                        "hover:bg-muted/50 flex w-full flex-col gap-0.5 px-3 py-3 text-left transition-colors",
                        active && "bg-muted",
                      )}
                    >
                      <span className="flex items-center gap-2 text-sm font-medium">
                        <span className="text-muted-foreground tabular-nums">
                          {channelDisplayNumber(channel)}
                        </span>
                        <span className="truncate">{channel.callsign || channel.name}</span>
                        {channel.hd ? (
                          <Badge variant="outline" className="ml-auto shrink-0">
                            HD
                          </Badge>
                        ) : null}
                      </span>
                      <span className="text-muted-foreground truncate text-xs">
                        {guideSlot.now?.title ?? "No guide data"}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </aside>

          <section className="min-w-0 space-y-6">
            {selected ? (
              <>
                <div className="space-y-3">
                  <div className="flex flex-wrap items-end justify-between gap-3">
                    <div>
                      <p className="text-muted-foreground text-xs tracking-wide uppercase">
                        Selected
                      </p>
                      <h2 className="text-2xl font-semibold">{channelLabel(selected)}</h2>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {activeSessionId ? (
                        <Button
                          variant="outline"
                          onClick={() => void onStop()}
                          disabled={releaseSession.isPending}
                        >
                          <Square />
                          Stop
                        </Button>
                      ) : (
                        <Button onClick={() => void onWatch()} disabled={startSession.isPending}>
                          <CircleDot />
                          {startSession.isPending ? "Starting…" : "Watch"}
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        onClick={onRecordNow}
                        disabled={!selectedGuide.now || scheduleRecording.isPending}
                      >
                        Record now
                      </Button>
                      <Button
                        variant="outline"
                        onClick={onRecordNext}
                        disabled={!selectedGuide.next || scheduleRecording.isPending}
                      >
                        Record next
                      </Button>
                    </div>
                  </div>

                  <div className="grid gap-4 sm:grid-cols-2">
                    <GuideSlot
                      label="Now"
                      title={selectedGuide.now?.title}
                      start={selectedGuide.now?.start}
                      stop={selectedGuide.now?.stop}
                      empty="Nothing airing right now"
                    />
                    <GuideSlot
                      label="Next"
                      title={selectedGuide.next?.title}
                      start={selectedGuide.next?.start}
                      stop={selectedGuide.next?.stop}
                      empty="No upcoming programme in this window"
                    />
                  </div>

                  {streamURL ? (
                    <p className="text-muted-foreground break-all text-xs">
                      Stream URL:{" "}
                      <a className="text-primary underline" href={streamURL} target="_blank" rel="noreferrer">
                        {streamURL}
                      </a>
                    </p>
                  ) : null}
                </div>

                <div className="space-y-3">
                  <h3 className="text-lg font-medium">Tonight on this channel</h3>
                  {guide.isLoading ? (
                    <p className="text-muted-foreground text-sm">Loading guide…</p>
                  ) : (
                    <ul className="divide-border divide-y border-y">
                      {programs
                        .filter((p) => p.channel_id === selected.id)
                        .map((program) => (
                          <li
                            key={program.id}
                            className="flex flex-wrap items-center justify-between gap-3 py-3"
                          >
                            <div className="min-w-0">
                              <p className="font-medium">{program.title}</p>
                              <p className="text-muted-foreground text-xs">
                                {formatGuideTime(program.start)} – {formatGuideTime(program.stop)}
                                {program.subtitle ? ` · ${program.subtitle}` : ""}
                              </p>
                            </div>
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={scheduleRecording.isPending}
                              onClick={() => scheduleRecording.mutate({ program_id: program.id })}
                            >
                              Record
                            </Button>
                          </li>
                        ))}
                      {programs.filter((p) => p.channel_id === selected.id).length === 0 ? (
                        <li className="text-muted-foreground py-4 text-sm">
                          No guide entries for this channel yet.
                        </li>
                      ) : null}
                    </ul>
                  )}
                </div>
              </>
            ) : null}

            <div className="space-y-3">
              <h3 className="text-lg font-medium">Scheduled recordings</h3>
              {recordings.isLoading ? (
                <p className="text-muted-foreground text-sm">Loading recordings…</p>
              ) : (recordings.data?.length ?? 0) === 0 ? (
                <p className="text-muted-foreground text-sm">No recordings scheduled.</p>
              ) : (
                <ul className="divide-border divide-y border-y">
                  {recordings.data?.map((rec) => (
                    <li
                      key={rec.id}
                      className="flex flex-wrap items-center justify-between gap-3 py-3"
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{rec.title || "Untitled"}</span>
                          <Badge variant="secondary">{rec.status}</Badge>
                        </div>
                        <p className="text-muted-foreground text-xs">
                          {formatGuideTime(rec.start)} – {formatGuideTime(rec.stop)}
                        </p>
                      </div>
                      {rec.status === "scheduled" ? (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={cancelRecording.isPending}
                          onClick={() => cancelRecording.mutate(rec.id)}
                        >
                          Cancel
                        </Button>
                      ) : null}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </section>
        </div>
      )}
    </div>
  );
}

function GuideSlot({
  label,
  title,
  start,
  stop,
  empty,
}: {
  label: string;
  title?: string;
  start?: string;
  stop?: string;
  empty: string;
}) {
  return (
    <div className="border-border space-y-1 border-t pt-3">
      <p className="text-muted-foreground text-[10px] font-semibold tracking-[0.22em] uppercase">
        {label}
      </p>
      {title ? (
        <>
          <p className="text-base font-medium">{title}</p>
          <p className="text-muted-foreground text-xs">
            {formatGuideTime(start ?? "")} – {formatGuideTime(stop ?? "")}
          </p>
        </>
      ) : (
        <p className="text-muted-foreground text-sm">{empty}</p>
      )}
    </div>
  );
}
