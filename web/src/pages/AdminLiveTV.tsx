import { useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { Plus, Radar, RefreshCw, Trash2 } from "lucide-react";
import type { LiveTVDiscoveredTuner } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useAddLiveTVTuner,
  useCancelLiveTVRecording,
  useCreateLiveTVGuideSource,
  useDeleteLiveTVGuideSource,
  useDeleteLiveTVTuner,
  useDiscoverLiveTVTuners,
  useLiveTVChannels,
  useLiveTVGuideSources,
  useLiveTVRecordings,
  useLiveTVTuners,
  usePatchLiveTVChannel,
  useScanLiveTVTuner,
  useSyncLiveTVGuideSource,
  useUpdateLiveTVGuideSource,
} from "@/hooks/queries/useLiveTV";

const LIVETV_TABS = ["tuners", "channels", "guide", "recordings"] as const;
type LiveTVTab = (typeof LIVETV_TABS)[number];

function normalizeTab(value: string | null): LiveTVTab {
  return LIVETV_TABS.includes(value as LiveTVTab) ? (value as LiveTVTab) : "tuners";
}

function kindLabel(kind: string): string {
  if (kind === "dispatcharr") return "Dispatcharr";
  if (kind === "hdhomerun") return "HDHomeRun";
  return kind || "Tuner";
}

function TunersTab() {
  const tuners = useLiveTVTuners();
  const addTuner = useAddLiveTVTuner();
  const discoverTuners = useDiscoverLiveTVTuners();
  const scanTuner = useScanLiveTVTuner();
  const deleteTuner = useDeleteLiveTVTuner();
  const [discoverURL, setDiscoverURL] = useState("");
  const [deviceID, setDeviceID] = useState("");
  const [probeURL, setProbeURL] = useState("");
  const [candidates, setCandidates] = useState<LiveTVDiscoveredTuner[]>([]);
  const [discoverNotes, setDiscoverNotes] = useState<string[]>([]);

  function submit() {
    addTuner.mutate(
      {
        discover_url: discoverURL.trim() || undefined,
        device_id: deviceID.trim() || undefined,
      },
      {
        onSuccess: () => {
          setDiscoverURL("");
          setDeviceID("");
        },
      },
    );
  }

  function runDiscovery(includeUDP: boolean) {
    const probe = probeURL.trim();
    discoverTuners.mutate(
      {
        timeout_ms: 2500,
        include_udp: includeUDP,
        probe_urls: probe ? [probe] : undefined,
      },
      {
        onSuccess: (data) => {
          setCandidates(data.candidates ?? []);
          setDiscoverNotes(data.notes ?? []);
        },
      },
    );
  }

  return (
    <div className="space-y-6">
      <div className="max-w-2xl space-y-4">
        <p className="text-muted-foreground text-sm">
          Auto-discover SiliconDust HDHomeRun tuners on the LAN (UDP) and probe a Dispatcharr URL
          for its HDHomeRun emulation (`/hdhr/discover.json`). Prairie scans the lineup after you
          add a candidate.
        </p>
        <div className="space-y-1.5">
          <Label htmlFor="probe-url">Dispatcharr / HDHR base URL (optional probe)</Label>
          <Input
            id="probe-url"
            placeholder="http://dispatcharr.local:9191 or http://192.168.1.50"
            value={probeURL}
            onChange={(e) => setProbeURL(e.target.value)}
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="secondary"
            onClick={() => runDiscovery(true)}
            disabled={discoverTuners.isPending}
          >
            <Radar />
            {discoverTuners.isPending ? "Discovering…" : "Discover on LAN"}
          </Button>
          <Button
            variant="outline"
            onClick={() => runDiscovery(false)}
            disabled={discoverTuners.isPending || !probeURL.trim()}
          >
            <Radar />
            Probe URL only
          </Button>
        </div>
        {discoverNotes.length > 0 ? (
          <ul className="text-muted-foreground list-disc space-y-1 pl-5 text-xs">
            {discoverNotes.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
        ) : null}
        {candidates.length > 0 ? (
          <ul className="divide-border divide-y border-y">
            {candidates.map((c) => (
              <li
                key={`${c.device_id}-${c.discover_url}`}
                className="flex flex-wrap items-center justify-between gap-3 py-3"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">
                      {c.friendly_name || c.model || c.device_id || "Tuner"}
                    </span>
                    <Badge variant="secondary">{kindLabel(c.kind)}</Badge>
                    <Badge variant="outline">{c.source}</Badge>
                    {c.tuner_count > 0 ? (
                      <Badge variant="outline">{c.tuner_count} tuners</Badge>
                    ) : null}
                    {c.already_added ? <Badge>Added</Badge> : null}
                  </div>
                  <p className="text-muted-foreground truncate text-xs">
                    {c.device_id}
                    {c.base_url ? ` · ${c.base_url}` : ""}
                  </p>
                </div>
                <Button
                  size="sm"
                  disabled={c.already_added || addTuner.isPending || !c.discover_url}
                  onClick={() =>
                    addTuner.mutate(
                      { discover_url: c.discover_url },
                      {
                        onSuccess: () => {
                          setCandidates((prev) =>
                            prev.map((row) =>
                              row.discover_url === c.discover_url
                                ? { ...row, already_added: true }
                                : row,
                            ),
                          );
                        },
                      },
                    )
                  }
                >
                  <Plus />
                  Add
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
      </div>

      <div className="max-w-xl space-y-4">
        <p className="text-muted-foreground text-sm">
          Or add manually by discover URL (preferred) or device host / ID.
        </p>
        <div className="space-y-1.5">
          <Label htmlFor="discover-url">Discover URL</Label>
          <Input
            id="discover-url"
            placeholder="http://192.168.1.50/discover.json"
            value={discoverURL}
            onChange={(e) => setDiscoverURL(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="device-id">Device ID / host (optional)</Label>
          <Input
            id="device-id"
            placeholder="ABCDEF01 or 192.168.1.50"
            value={deviceID}
            onChange={(e) => setDeviceID(e.target.value)}
          />
        </div>
        <Button
          onClick={submit}
          disabled={addTuner.isPending || (!discoverURL.trim() && !deviceID.trim())}
        >
          <Plus />
          {addTuner.isPending ? "Adding…" : "Add tuner"}
        </Button>
      </div>

      {tuners.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading tuners…</p>
      ) : (tuners.data?.length ?? 0) === 0 ? (
        <p className="text-muted-foreground text-sm">No tuners configured yet.</p>
      ) : (
        <ul className="divide-border divide-y border-y">
          {tuners.data?.map((tuner) => (
            <li key={tuner.id} className="flex flex-wrap items-center justify-between gap-3 py-4">
              <div className="min-w-0 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{tuner.model || tuner.device_id}</span>
                  <Badge variant="secondary">{tuner.status}</Badge>
                  <Badge variant="outline">{tuner.channel_count} channels</Badge>
                </div>
                <p className="text-muted-foreground truncate text-xs">
                  {tuner.device_id}
                  {tuner.base_url ? ` · ${tuner.base_url}` : ""}
                </p>
                {tuner.last_error ? (
                  <p className="text-destructive text-xs">{tuner.last_error}</p>
                ) : null}
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={scanTuner.isPending}
                  onClick={() => scanTuner.mutate(tuner.id)}
                >
                  <RefreshCw />
                  Rescan
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={deleteTuner.isPending}
                  onClick={() => deleteTuner.mutate(tuner.id)}
                >
                  <Trash2 />
                  Remove
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function ChannelsTab() {
  const channels = useLiveTVChannels();
  const patchChannel = usePatchLiveTVChannel();
  const [stationDrafts, setStationDrafts] = useState<Record<string, string>>({});

  const sorted = useMemo(
    () =>
      [...(channels.data ?? [])].sort((a, b) =>
        (a.number_override || a.number).localeCompare(b.number_override || b.number, undefined, {
          numeric: true,
        }),
      ),
    [channels.data],
  );

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        Enable channels for Live TV, override display numbers, and map each channel to a guide
        station ID from your XMLTV / Schedules Direct source.
      </p>
      {channels.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading channels…</p>
      ) : sorted.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No channels yet. Add and scan a tuner first.
        </p>
      ) : (
        <ul className="divide-border divide-y border-y">
          {sorted.map((channel) => {
            const stationValue = stationDrafts[channel.id] ?? channel.guide_station_id ?? "";
            return (
              <li key={channel.id} className="grid gap-3 py-4 sm:grid-cols-[1fr_auto]">
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">
                      {channel.number_override || channel.number} ·{" "}
                      {channel.callsign || channel.name}
                    </span>
                    {channel.hd ? <Badge variant="secondary">HD</Badge> : null}
                  </div>
                  <div className="flex flex-wrap items-end gap-3">
                    <div className="space-y-1">
                      <Label htmlFor={`station-${channel.id}`} className="text-xs">
                        Guide station ID
                      </Label>
                      <Input
                        id={`station-${channel.id}`}
                        className="w-48"
                        value={stationValue}
                        onChange={(e) =>
                          setStationDrafts((prev) => ({ ...prev, [channel.id]: e.target.value }))
                        }
                        onBlur={() => {
                          if (stationValue === (channel.guide_station_id || "")) return;
                          patchChannel.mutate({
                            channelId: channel.id,
                            body: { guide_station_id: stationValue },
                          });
                        }}
                      />
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2 self-start sm:self-center">
                  <Label
                    htmlFor={`enabled-${channel.id}`}
                    className="text-muted-foreground text-sm"
                  >
                    Enabled
                  </Label>
                  <Switch
                    id={`enabled-${channel.id}`}
                    checked={channel.enabled}
                    disabled={patchChannel.isPending}
                    onCheckedChange={(checked) =>
                      patchChannel.mutate({
                        channelId: channel.id,
                        body: { enabled: checked },
                      })
                    }
                  />
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function GuideTab() {
  const sources = useLiveTVGuideSources();
  const createSource = useCreateLiveTVGuideSource();
  const updateSource = useUpdateLiveTVGuideSource();
  const deleteSource = useDeleteLiveTVGuideSource();
  const syncSource = useSyncLiveTVGuideSource();
  const [displayName, setDisplayName] = useState("XMLTV");
  const [xmltvURL, setXmltvURL] = useState("");

  function addXMLTV() {
    createSource.mutate(
      {
        type: "xmltv_url",
        enabled: true,
        display_name: displayName.trim() || "XMLTV",
        priority: 100,
        config: { url: xmltvURL.trim() },
      },
      {
        onSuccess: () => {
          setXmltvURL("");
        },
      },
    );
  }

  return (
    <div className="space-y-6">
      <div className="max-w-xl space-y-4">
        <p className="text-muted-foreground text-sm">
          Up to three enabled guide sources, priority-ordered like marker providers. XMLTV URL sync
          works now; Schedules Direct lands next.
        </p>
        <div className="space-y-1.5">
          <Label htmlFor="guide-name">Display name</Label>
          <Input
            id="guide-name"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="xmltv-url">XMLTV URL</Label>
          <Input
            id="xmltv-url"
            placeholder="https://example.com/guide.xml"
            value={xmltvURL}
            onChange={(e) => setXmltvURL(e.target.value)}
          />
        </div>
        <Button
          onClick={addXMLTV}
          disabled={
            createSource.isPending ||
            !xmltvURL.trim() ||
            (sources.data?.filter((s) => s.enabled).length ?? 0) >= 3
          }
        >
          <Plus />
          {createSource.isPending ? "Adding…" : "Add XMLTV source"}
        </Button>
      </div>

      {sources.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading guide sources…</p>
      ) : (sources.data?.length ?? 0) === 0 ? (
        <p className="text-muted-foreground text-sm">No guide sources configured.</p>
      ) : (
        <ul className="divide-border divide-y border-y">
          {sources.data?.map((source) => (
            <li key={source.id} className="flex flex-wrap items-center justify-between gap-3 py-4">
              <div className="min-w-0 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{source.display_name || source.type}</span>
                  <Badge variant="secondary">{source.type}</Badge>
                  <Badge variant="outline">priority {source.priority}</Badge>
                  <Badge variant="outline">{source.status}</Badge>
                </div>
                <p className="text-muted-foreground truncate text-xs">
                  {source.config?.url || "No URL"}
                  {source.last_sync_at
                    ? ` · last sync ${new Date(source.last_sync_at).toLocaleString()}`
                    : ""}
                </p>
                {source.last_error ? (
                  <p className="text-destructive text-xs">{source.last_error}</p>
                ) : null}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex items-center gap-2">
                  <Label
                    htmlFor={`source-enabled-${source.id}`}
                    className="text-muted-foreground text-sm"
                  >
                    Enabled
                  </Label>
                  <Switch
                    id={`source-enabled-${source.id}`}
                    checked={source.enabled}
                    disabled={updateSource.isPending}
                    onCheckedChange={(checked) =>
                      updateSource.mutate({
                        id: source.id,
                        body: { ...source, enabled: checked },
                      })
                    }
                  />
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={syncSource.isPending}
                  onClick={() => syncSource.mutate(source.id)}
                >
                  <RefreshCw />
                  Sync
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={deleteSource.isPending}
                  onClick={() => deleteSource.mutate(source.id)}
                >
                  <Trash2 />
                  Remove
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function RecordingsTab() {
  const recordings = useLiveTVRecordings();
  const cancelRecording = useCancelLiveTVRecording();

  if (recordings.isLoading) {
    return <p className="text-muted-foreground text-sm">Loading recordings…</p>;
  }
  if ((recordings.data?.length ?? 0) === 0) {
    return <p className="text-muted-foreground text-sm">No recordings yet.</p>;
  }

  return (
    <ul className="divide-border divide-y border-y">
      {recordings.data?.map((rec) => (
        <li key={rec.id} className="flex flex-wrap items-center justify-between gap-3 py-4">
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{rec.title || "Untitled"}</span>
              <Badge variant="secondary">{rec.status}</Badge>
            </div>
            <p className="text-muted-foreground text-xs">
              {new Date(rec.start).toLocaleString()} – {new Date(rec.stop).toLocaleString()}
              {rec.last_error ? ` · ${rec.last_error}` : ""}
            </p>
          </div>
          {rec.status === "scheduled" ? (
            <Button
              variant="outline"
              size="sm"
              disabled={cancelRecording.isPending}
              onClick={() => cancelRecording.mutate(rec.id)}
            >
              Cancel
            </Button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

export default function AdminLiveTV() {
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = normalizeTab(searchParams.get("tab"));
  const tuners = useLiveTVTuners();
  const channels = useLiveTVChannels();
  const sources = useLiveTVGuideSources();

  function setActiveTab(value: string) {
    const next = new URLSearchParams(searchParams);
    if (value === "tuners") {
      next.delete("tab");
    } else {
      next.set("tab", value);
    }
    setSearchParams(next, { replace: true });
  }

  return (
    <div className="space-y-6">
      <div className="page-header">
        <div className="space-y-2">
          <div className="flex items-center gap-2.5">
            <h1 className="text-3xl font-semibold tracking-normal text-balance sm:text-4xl">
              Live TV
            </h1>
            <Badge variant="secondary">{tuners.data?.length ?? 0} tuners</Badge>
            <Badge variant="outline">{channels.data?.length ?? 0} channels</Badge>
            <Badge variant="outline">
              {sources.data?.filter((s) => s.enabled).length ?? 0}/3 guides
            </Badge>
          </div>
          <p className="text-muted-foreground max-w-2xl text-sm leading-6">
            Configure HDHomeRun OTA tuners, map channels to guide stations, and keep the EPG fresh
            with priority-ordered guide sources.
          </p>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="gap-5">
        <TabsList variant="line" className="border-border w-full justify-start border-b">
          <TabsTrigger value="tuners">Tuners</TabsTrigger>
          <TabsTrigger value="channels">Channels</TabsTrigger>
          <TabsTrigger value="guide">Guide sources</TabsTrigger>
          <TabsTrigger value="recordings">Recordings</TabsTrigger>
        </TabsList>
        <TabsContent value="tuners">
          <TunersTab />
        </TabsContent>
        <TabsContent value="channels">
          <ChannelsTab />
        </TabsContent>
        <TabsContent value="guide">
          <GuideTab />
        </TabsContent>
        <TabsContent value="recordings">
          <RecordingsTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
