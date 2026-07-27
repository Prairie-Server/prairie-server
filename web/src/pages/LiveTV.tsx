import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { Circle, Play, Radio, Square, X } from "lucide-react";
import { toast } from "sonner";
import type { LiveTVChannel, LiveTVRecording } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { LiveTVGuideGrid } from "@/components/livetv/LiveTVGuideGrid";
import { LiveTVPlayer } from "@/components/livetv/LiveTVPlayer";
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
  buildGuideWindow,
  channelDisplayNumber,
  channelLabel,
  formatGuideTime,
  pickNowNext,
  progressFraction,
} from "@/lib/liveTVGuide";
import { cn } from "@/lib/utils";

const LIVETV_TABS = ["guide", "channels", "recordings"] as const;
type LiveTVTab = (typeof LIVETV_TABS)[number];

function normalizeTab(value: string | null): LiveTVTab {
  return LIVETV_TABS.includes(value as LiveTVTab) ? (value as LiveTVTab) : "guide";
}

export default function LiveTV() {
  useDocumentTitle("Live TV");
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = normalizeTab(searchParams.get("tab"));
  const channelFromUrl = searchParams.get("channel");
  const shouldAutoWatch = searchParams.get("watch") === "1";
  const channelsQuery = useLiveTVChannels();
  const channels = useMemo(
    () => (channelsQuery.data ?? []).filter((ch) => ch.enabled),
    [channelsQuery.data],
  );
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [channelFilter, setChannelFilter] = useState("");
  const selected = channels.find((ch) => ch.id === selectedId) ?? channels[0] ?? null;

  useEffect(() => {
    if (channelFromUrl && channels.some((ch) => ch.id === channelFromUrl)) {
      setSelectedId(channelFromUrl);
      return;
    }
    if (!selectedId && channels[0]) {
      setSelectedId(channels[0].id);
    }
  }, [channels, selectedId, channelFromUrl]);

  // Refresh the guide window periodically so "now" stays accurate.
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNowMs(Date.now()), 60_000);
    return () => window.clearInterval(id);
  }, []);
  const now = useMemo(() => new Date(nowMs), [nowMs]);

  const guideWindow = useMemo(() => {
    const win = buildGuideWindow(new Date(nowMs));
    return {
      start: new Date(win.startMs).toISOString(),
      end: new Date(win.endMs).toISOString(),
    };
  }, [nowMs]);

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
  const [streamTransport, setStreamTransport] = useState<"mpegts" | "hls">("mpegts");
  const [watchingChannelId, setWatchingChannelId] = useState<string | null>(null);
  const [startingChannelId, setStartingChannelId] = useState<string | null>(null);
  const autoWatchAttempted = useRef<string | null>(null);
  const playerSectionRef = useRef<HTMLElement | null>(null);

  const selectedGuide = selected
    ? pickNowNext(programs, selected.id, now)
    : { now: null, next: null };
  const watchingChannel =
    channels.find((ch) => ch.id === watchingChannelId) ?? (watchingChannelId ? selected : null);
  const filteredChannels = useMemo(() => {
    const q = channelFilter.trim().toLowerCase();
    if (!q) return channels;
    return channels.filter((ch) => {
      const hay = `${channelDisplayNumber(ch)} ${ch.callsign} ${ch.name}`.toLowerCase();
      return hay.includes(q);
    });
  }, [channelFilter, channels]);

  function setTab(tab: string) {
    const next = normalizeTab(tab);
    const params = new URLSearchParams(searchParams);
    if (next === "guide") params.delete("tab");
    else params.set("tab", next);
    setSearchParams(params, { replace: true });
  }

  function clearWatchParam() {
    if (!shouldAutoWatch) return;
    const params = new URLSearchParams(searchParams);
    params.delete("watch");
    setSearchParams(params, { replace: true });
  }

  async function onWatch(channelId?: string) {
    const targetId = channelId ?? selected?.id;
    if (!targetId || startingChannelId) return;
    const channel = channels.find((ch) => ch.id === targetId) ?? selected;
    if (!channel) return;
    setStartingChannelId(channel.id);
    setSelectedId(channel.id);
    try {
      if (activeSessionId) {
        try {
          await releaseSession.mutateAsync(activeSessionId);
        } catch {
          // Continue — a new tune should still attempt to start.
        }
      }
      const session = await startSession.mutateAsync(channel.id);
      const nextUrl = session.hls_url || session.stream_url || null;
      setActiveSessionId(session.session_id);
      setWatchingChannelId(channel.id);
      setStreamURL(nextUrl);
      setStreamTransport(session.transport === "hls" ? "hls" : "mpegts");
      if (!nextUrl) {
        toast.error("Live TV session started but no stream URL was returned");
      } else if (session.note) {
        toast.message(session.note);
      } else {
        toast.success(`Watching ${channelLabel(channel)}`);
      }
      requestAnimationFrame(() => {
        playerSectionRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
      });
    } catch {
      // useStartLiveTVSession already toasts.
    } finally {
      setStartingChannelId(null);
    }
  }

  useEffect(() => {
    if (!shouldAutoWatch || !channelFromUrl || channels.length === 0) return;
    if (!channels.some((ch) => ch.id === channelFromUrl)) return;
    if (autoWatchAttempted.current === channelFromUrl) return;
    autoWatchAttempted.current = channelFromUrl;
    void onWatch(channelFromUrl).finally(() => {
      clearWatchParam();
    });
    // Intentionally once per channel deep-link; onWatch closes over latest channels/session.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shouldAutoWatch, channelFromUrl, channels]);

  async function onStop() {
    if (!activeSessionId) {
      setStreamURL(null);
      setWatchingChannelId(null);
      return;
    }
    try {
      await releaseSession.mutateAsync(activeSessionId);
      toast.success("Live TV session released");
    } catch {
      // useReleaseLiveTVSession already toasts.
    } finally {
      setActiveSessionId(null);
      setStreamURL(null);
      setStreamTransport("mpegts");
      setWatchingChannelId(null);
    }
  }

  const scheduled = (recordings.data ?? []).filter((r) =>
    ["scheduled", "recording"].includes(r.status),
  );
  const history = (recordings.data ?? []).filter(
    (r) => !["scheduled", "recording"].includes(r.status),
  );

  return (
    <div className="mx-auto flex w-full max-w-[100rem] flex-col gap-6 px-4 py-6 sm:px-6">
      <header className="space-y-1">
        <div className="flex flex-wrap items-center gap-3">
          <Radio className="text-primary h-7 w-7" aria-hidden />
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Live TV</h1>
          <Badge variant="secondary">{channels.length} channels</Badge>
        </div>
        <p className="text-muted-foreground max-w-2xl text-sm leading-6">
          Guide grid, channel lineup, and your recordings — watch or schedule from a programme or
          channel row.
        </p>
      </header>

      {streamURL ? (
        <section
          ref={playerSectionRef}
          className="border-border overflow-hidden rounded-xl border"
        >
          <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2">
            <div className="min-w-0">
              <p className="text-muted-foreground text-[10px] font-semibold tracking-[0.18em] uppercase">
                Now watching
              </p>
              <p className="truncate font-medium">
                {watchingChannel ? channelLabel(watchingChannel) : "Live"}
                {selectedGuide.now && watchingChannelId === selected?.id
                  ? ` · ${selectedGuide.now.title}`
                  : ""}
              </p>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void onStop()}
              disabled={releaseSession.isPending}
            >
              <Square />
              Stop
            </Button>
          </div>
          <LiveTVPlayer
            streamUrl={streamURL}
            transport={streamTransport}
            title={watchingChannel ? channelLabel(watchingChannel) : "Live TV"}
            className="aspect-video w-full"
          />
        </section>
      ) : null}

      {channelsQuery.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading channels…</p>
      ) : channels.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No Live TV channels yet. An admin can add an HDHomeRun tuner under Admin → Live TV.
        </p>
      ) : (
        <Tabs value={activeTab} onValueChange={setTab} className="gap-5">
          <TabsList variant="line" className="border-border w-full justify-start border-b">
            <TabsTrigger value="guide">Guide</TabsTrigger>
            <TabsTrigger value="channels">Channels</TabsTrigger>
            <TabsTrigger value="recordings">
              My recordings
              {scheduled.length > 0 ? (
                <Badge variant="secondary" className="ml-1.5">
                  {scheduled.length}
                </Badge>
              ) : null}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="guide" className="space-y-4">
            {guide.isLoading ? (
              <p className="text-muted-foreground text-sm">Loading guide…</p>
            ) : (
              <LiveTVGuideGrid
                channels={channels}
                programs={programs}
                selectedChannelId={selected?.id ?? null}
                now={now}
                onSelectChannel={setSelectedId}
                onWatch={(id) => void onWatch(id)}
                onRecord={(programId) => scheduleRecording.mutate({ program_id: programId })}
                recordDisabled={scheduleRecording.isPending}
                startingChannelId={startingChannelId}
              />
            )}
          </TabsContent>

          <TabsContent value="channels" className="space-y-4">
            <div className="flex flex-wrap items-center gap-3">
              <Input
                value={channelFilter}
                onChange={(e) => setChannelFilter(e.target.value)}
                placeholder="Filter channels…"
                className="max-w-sm"
              />
              <p className="text-muted-foreground text-xs">
                {filteredChannels.length} of {channels.length}
              </p>
            </div>
            <ul className="divide-border divide-y border-y">
              {filteredChannels.map((channel) => (
                <ChannelListRow
                  key={channel.id}
                  channel={channel}
                  programs={programs}
                  now={now}
                  active={selected?.id === channel.id}
                  watching={watchingChannelId === channel.id}
                  watchBusy={startingChannelId === channel.id}
                  recordBusy={scheduleRecording.isPending}
                  onSelect={() => setSelectedId(channel.id)}
                  onWatch={() => void onWatch(channel.id)}
                  onRecordNow={(programId) => scheduleRecording.mutate({ program_id: programId })}
                  onRecordNext={(programId) => scheduleRecording.mutate({ program_id: programId })}
                />
              ))}
              {filteredChannels.length === 0 ? (
                <li className="text-muted-foreground py-6 text-sm">
                  No channels match that filter.
                </li>
              ) : null}
            </ul>
          </TabsContent>

          <TabsContent value="recordings" className="space-y-8">
            <RecordingsSection
              title="Scheduled & in progress"
              empty="Nothing scheduled yet. Pick a programme from the guide or channel list."
              recordings={scheduled}
              loading={recordings.isLoading}
              channels={channels}
              cancelRecording={cancelRecording}
            />
            <RecordingsSection
              title="History"
              empty="Completed and failed recordings will show up here."
              recordings={history}
              loading={false}
              channels={channels}
              cancelRecording={cancelRecording}
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function ChannelListRow({
  channel,
  programs,
  now,
  active,
  watching,
  watchBusy,
  recordBusy,
  onSelect,
  onWatch,
  onRecordNow,
  onRecordNext,
}: {
  channel: LiveTVChannel;
  programs: Array<{
    id: string;
    channel_id: string;
    title: string;
    start: string;
    stop: string;
  }>;
  now: Date;
  active: boolean;
  watching: boolean;
  watchBusy: boolean;
  recordBusy: boolean;
  onSelect: () => void;
  onWatch: () => void;
  onRecordNow: (programId: string) => void;
  onRecordNext: (programId: string) => void;
}) {
  const slot = pickNowNext(programs, channel.id, now);
  const progress = slot.now ? progressFraction(slot.now.start, slot.now.stop, now) : 0;

  return (
    <li>
      <div
        className={cn(
          "flex flex-col gap-3 py-3 sm:flex-row sm:items-center sm:justify-between",
          active && "bg-muted/40 -mx-2 rounded-lg px-2",
        )}
      >
        <button type="button" onClick={onSelect} className="min-w-0 flex-1 text-left">
          <div className="flex items-center gap-3">
            {channel.logo_url ? (
              <img
                src={channel.logo_url}
                alt=""
                className="bg-muted h-10 w-10 rounded object-contain"
              />
            ) : (
              <div className="bg-muted text-muted-foreground flex h-10 w-10 items-center justify-center rounded text-xs font-semibold">
                {channelDisplayNumber(channel)}
              </div>
            )}
            <div className="min-w-0">
              <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                <span className="text-muted-foreground tabular-nums">
                  {channelDisplayNumber(channel)}
                </span>
                <span className="truncate">{channel.callsign || channel.name}</span>
                {channel.hd ? <Badge variant="outline">HD</Badge> : null}
                {watching ? <Badge>Live</Badge> : null}
              </p>
              <p className="text-muted-foreground truncate text-xs">
                {slot.now?.title ?? "No guide data"}
                {slot.next ? ` → ${slot.next.title}` : ""}
              </p>
              {slot.now ? (
                <div className="bg-muted mt-1.5 h-1 w-full max-w-xs overflow-hidden rounded-full">
                  <div className="bg-primary h-full" style={{ width: `${progress * 100}%` }} />
                </div>
              ) : null}
            </div>
          </div>
        </button>
        <div className="flex flex-wrap gap-2 sm:shrink-0">
          <Button type="button" size="sm" onClick={onWatch} disabled={watchBusy}>
            <Play />
            Watch
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!slot.now || recordBusy}
            onClick={() => slot.now && onRecordNow(slot.now.id)}
          >
            <Circle />
            Record now
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!slot.next || recordBusy}
            onClick={() => slot.next && onRecordNext(slot.next.id)}
          >
            <Circle />
            Record next
          </Button>
        </div>
      </div>
    </li>
  );
}

function RecordingsSection({
  title,
  empty,
  recordings,
  loading,
  channels,
  cancelRecording,
}: {
  title: string;
  empty: string;
  recordings: LiveTVRecording[];
  loading: boolean;
  channels: LiveTVChannel[];
  cancelRecording: ReturnType<typeof useCancelLiveTVRecording>;
}) {
  const channelById = useMemo(() => {
    const map = new Map<string, LiveTVChannel>();
    for (const ch of channels) map.set(ch.id, ch);
    return map;
  }, [channels]);

  return (
    <div className="space-y-3">
      <h2 className="text-lg font-medium">{title}</h2>
      {loading ? (
        <p className="text-muted-foreground text-sm">Loading recordings…</p>
      ) : recordings.length === 0 ? (
        <p className="text-muted-foreground text-sm">{empty}</p>
      ) : (
        <ul className="divide-border divide-y border-y">
          {recordings.map((rec) => {
            const channel = channelById.get(rec.channel_id);
            return (
              <li key={rec.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{rec.title || "Untitled"}</span>
                    <Badge variant="secondary">{rec.status}</Badge>
                  </div>
                  <p className="text-muted-foreground text-xs">
                    {channel ? channelLabel(channel) : rec.channel_id}
                    {" · "}
                    {formatGuideTime(rec.start)} – {formatGuideTime(rec.stop)}
                  </p>
                  {rec.last_error ? (
                    <p className="text-destructive mt-1 text-xs">{rec.last_error}</p>
                  ) : null}
                  {rec.library_item_id ? (
                    <p className="text-muted-foreground mt-1 text-xs">
                      Library item {rec.library_item_id}
                    </p>
                  ) : null}
                </div>
                {rec.status === "scheduled" ? (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={cancelRecording.isPending}
                    onClick={() => cancelRecording.mutate(rec.id)}
                  >
                    <X />
                    Cancel
                  </Button>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
