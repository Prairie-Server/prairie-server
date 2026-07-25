import { useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { Plus, RefreshCw, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useAddLiveTVTuner,
  useCreateLiveTVGuideSource,
  useDeleteLiveTVGuideSource,
  useDeleteLiveTVTuner,
  useLiveTVChannels,
  useLiveTVGuideSources,
  useLiveTVTuners,
  usePatchLiveTVChannel,
  useScanLiveTVTuner,
  useSyncLiveTVGuideSource,
  useUpdateLiveTVGuideSource,
} from "@/hooks/queries/useLiveTV";

const LIVETV_TABS = ["tuners", "channels", "guide"] as const;
type LiveTVTab = (typeof LIVETV_TABS)[number];

function normalizeTab(value: string | null): LiveTVTab {
  return LIVETV_TABS.includes(value as LiveTVTab) ? (value as LiveTVTab) : "tuners";
}

function TunersTab() {
  const tuners = useLiveTVTuners();
  const addTuner = useAddLiveTVTuner();
  const scanTuner = useScanLiveTVTuner();
  const deleteTuner = useDeleteLiveTVTuner();
  const [discoverURL, setDiscoverURL] = useState("");
  const [deviceID, setDeviceID] = useState("");

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

  return (
    <div className="space-y-6">
      <div className="max-w-xl space-y-4">
        <p className="text-muted-foreground text-sm">
          Add an HDHomeRun by discover URL (preferred) or device ID. Prairie scans the lineup after
          discovery.
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
          <Label htmlFor="device-id">Device ID (optional)</Label>
          <Input
            id="device-id"
            placeholder="ABCDEF01"
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
        <p className="text-muted-foreground text-sm">No channels yet. Add and scan a tuner first.</p>
      ) : (
        <ul className="divide-border divide-y border-y">
          {sorted.map((channel) => {
            const stationValue =
              stationDrafts[channel.id] ?? channel.guide_station_id ?? "";
            return (
              <li key={channel.id} className="grid gap-3 py-4 sm:grid-cols-[1fr_auto]">
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">
                      {channel.number_override || channel.number} · {channel.callsign || channel.name}
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
                  <Label htmlFor={`enabled-${channel.id}`} className="text-muted-foreground text-sm">
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
          disabled={createSource.isPending || !xmltvURL.trim() || (sources.data?.filter((s) => s.enabled).length ?? 0) >= 3}
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
                  {source.last_sync_at ? ` · last sync ${new Date(source.last_sync_at).toLocaleString()}` : ""}
                </p>
                {source.last_error ? (
                  <p className="text-destructive text-xs">{source.last_error}</p>
                ) : null}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex items-center gap-2">
                  <Label htmlFor={`source-enabled-${source.id}`} className="text-muted-foreground text-sm">
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
            <Badge variant="outline">{sources.data?.filter((s) => s.enabled).length ?? 0}/3 guides</Badge>
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
      </Tabs>
    </div>
  );
}
