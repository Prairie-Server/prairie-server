import { useEffect } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import type {
  LiveTVChannel,
  LiveTVChannelsResponse,
  LiveTVGuideResponse,
  LiveTVGuideSource,
  LiveTVGuideSourcesResponse,
  LiveTVRecording,
  LiveTVRecordingsResponse,
  LiveTVSessionStartResponse,
  LiveTVDiscoverTunersResponse,
  LiveTVTuner,
  LiveTVTunersResponse,
  SchedulesDirectLineupsResponse,
  XMLSyncLineupsResponse,
} from "@/api/types";
import { adminKeys } from "./keys";

const LIVETV_STALE_TIME = 30_000;

/** Well inside the server's stale-session TTL so a paused player keeps its tuner. */
export const LIVETV_HEARTBEAT_INTERVAL_MS = 30_000;

export function useLiveTVTuners() {
  return useQuery({
    queryKey: adminKeys.liveTVTuners(),
    queryFn: () => api<LiveTVTunersResponse>("/livetv/tuners").then((data) => data.tuners ?? []),
    staleTime: LIVETV_STALE_TIME,
  });
}

export function useDiscoverLiveTVTuners() {
  return useMutation({
    mutationFn: (body: { timeout_ms?: number; include_udp?: boolean; probe_urls?: string[] }) =>
      api<LiveTVDiscoverTunersResponse>("/livetv/tuners/discover", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Tuner discovery failed");
    },
  });
}

export function useAddLiveTVTuner() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { url?: string; discover_url?: string; device_id?: string }) =>
      api<LiveTVTuner>("/livetv/tuners", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Tuner added");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVTuners() });
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to add tuner");
    },
  });
}

export function useScanLiveTVTuner() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (tunerId: string) =>
      api<LiveTVTuner>(`/livetv/tuners/${encodeURIComponent(tunerId)}/scan`, {
        method: "POST",
      }),
    onSuccess: () => {
      toast.success("Channel lineup rescanned");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVTuners() });
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to scan tuner");
    },
  });
}

export function useDeleteLiveTVTuner() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (tunerId: string) =>
      api(`/livetv/tuners/${encodeURIComponent(tunerId)}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Tuner removed");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVTuners() });
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to delete tuner");
    },
  });
}

export function useLiveTVChannels(tunerId?: string) {
  const params = new URLSearchParams();
  if (tunerId) params.set("tuner_id", tunerId);
  const qs = params.toString();
  return useQuery({
    queryKey: adminKeys.liveTVChannels(tunerId),
    queryFn: () =>
      api<LiveTVChannelsResponse>(`/livetv/channels${qs ? `?${qs}` : ""}`).then(
        (data) => data.channels ?? [],
      ),
    staleTime: LIVETV_STALE_TIME,
    placeholderData: keepPreviousData,
  });
}

export function usePatchLiveTVChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      channelId,
      body,
    }: {
      channelId: string;
      body: { enabled?: boolean; number_override?: string | null; guide_station_id?: string };
    }) =>
      api<LiveTVChannel>(`/livetv/channels/${encodeURIComponent(channelId)}`, {
        method: "PATCH",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Channel updated");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update channel");
    },
  });
}

export function useLiveTVGuideSources() {
  return useQuery({
    queryKey: adminKeys.liveTVGuideSources(),
    queryFn: () =>
      api<LiveTVGuideSourcesResponse>("/livetv/guide-sources").then(
        (data) => data.guide_sources ?? [],
      ),
    staleTime: LIVETV_STALE_TIME,
  });
}

export function useCreateLiveTVGuideSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<LiveTVGuideSource>) =>
      api<LiveTVGuideSource>("/livetv/guide-sources", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Guide source added");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to add guide source");
    },
  });
}

export function useUpdateLiveTVGuideSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<LiveTVGuideSource> }) =>
      api<LiveTVGuideSource>(`/livetv/guide-sources/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Guide source updated");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update guide source");
    },
  });
}

export function useDeleteLiveTVGuideSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api(`/livetv/guide-sources/${encodeURIComponent(id)}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Guide source removed");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to delete guide source");
    },
  });
}

export function useSyncLiveTVGuideSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<LiveTVGuideSource>(`/livetv/guide-sources/${encodeURIComponent(id)}/sync`, {
        method: "POST",
      }),
    onSuccess: () => {
      toast.success("Guide sync finished");
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
      void queryClient.invalidateQueries({ queryKey: ["livetv", "guide"] });
    },
    onError: (err) => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
      toast.error(err instanceof Error ? err.message : "Failed to sync guide source");
    },
  });
}

export function useLookupSchedulesDirectLineups() {
  return useMutation({
    mutationFn: (body: {
      username: string;
      password: string;
      country?: string;
      postalcode: string;
    }) =>
      api<SchedulesDirectLineupsResponse>("/livetv/guide-sources/schedules-direct/lineups", {
        method: "POST",
        body: JSON.stringify(body),
      }).then((data) => data.lineups ?? []),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to look up lineups");
    },
  });
}

export function useLookupXMLSyncLineups() {
  return useMutation({
    mutationFn: (body: { country?: string; postalcode: string; lang?: string }) =>
      api<XMLSyncLineupsResponse>("/livetv/guide-sources/xml-sync/lineups", {
        method: "POST",
        body: JSON.stringify(body),
      }).then((data) => data.lineups ?? []),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to look up XML sync lineups");
    },
  });
}

export type LiveTVGuideParams = {
  channelIds?: string[];
  start?: string;
  end?: string;
};

export function useLiveTVGuide(params: LiveTVGuideParams = {}, enabled = true) {
  const search = new URLSearchParams();
  if (params.channelIds?.length) search.set("channels", params.channelIds.join(","));
  if (params.start) search.set("start", params.start);
  if (params.end) search.set("end", params.end);
  const qs = search.toString();
  return useQuery({
    queryKey: adminKeys.liveTVGuide({
      channels: params.channelIds?.join(",") ?? "",
      start: params.start ?? "",
      end: params.end ?? "",
    }),
    queryFn: () =>
      api<LiveTVGuideResponse>(`/livetv/guide${qs ? `?${qs}` : ""}`).then((data) => ({
        programs: data.programs ?? [],
        start: data.start,
        end: data.end,
      })),
    staleTime: LIVETV_STALE_TIME,
    enabled,
    placeholderData: keepPreviousData,
  });
}

export function useStartLiveTVSession() {
  return useMutation({
    mutationFn: (channelId: string) =>
      api<LiveTVSessionStartResponse>(`/livetv/channels/${encodeURIComponent(channelId)}/session`, {
        method: "POST",
      }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to start Live TV session");
    },
  });
}

/**
 * Keeps a live session's tuner claimed while the player is open. The server
 * reclaims sessions that stop being watched, so a paused player (which stops
 * fetching segments) still needs to check in.
 */
export function useLiveTVSessionHeartbeat(sessionId: string | null, enabled = true) {
  useEffect(() => {
    if (!sessionId || !enabled) return;
    const send = () => {
      void api(`/livetv/sessions/${encodeURIComponent(sessionId)}/heartbeat`, {
        method: "POST",
      }).catch(() => undefined);
    };
    send();
    const id = window.setInterval(send, LIVETV_HEARTBEAT_INTERVAL_MS);
    return () => window.clearInterval(id);
  }, [sessionId, enabled]);
}

export function useReleaseLiveTVSession() {
  return useMutation({
    mutationFn: (sessionId: string) =>
      api(`/livetv/sessions/${encodeURIComponent(sessionId)}`, { method: "DELETE" }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to release Live TV session");
    },
  });
}

export function useLiveTVRecordings(status?: string) {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  const qs = params.toString();
  return useQuery({
    queryKey: adminKeys.liveTVRecordings(status),
    queryFn: () =>
      api<LiveTVRecordingsResponse>(`/livetv/recordings${qs ? `?${qs}` : ""}`).then(
        (data) => data.recordings ?? [],
      ),
    staleTime: LIVETV_STALE_TIME,
  });
}

export function useScheduleLiveTVRecording() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      program_id?: string;
      channel_id?: string;
      start?: string;
      stop?: string;
      title?: string;
    }) =>
      api<LiveTVRecording>("/livetv/recordings", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Recording scheduled");
      void queryClient.invalidateQueries({ queryKey: ["livetv", "recordings"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to schedule recording");
    },
  });
}

export function useCancelLiveTVRecording() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (recordingId: string) =>
      api<LiveTVRecording>(`/livetv/recordings/${encodeURIComponent(recordingId)}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      toast.success("Recording cancelled");
      void queryClient.invalidateQueries({ queryKey: ["livetv", "recordings"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to cancel recording");
    },
  });
}
