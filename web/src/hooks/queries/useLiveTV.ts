import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import type {
  LiveTVChannel,
  LiveTVChannelsResponse,
  LiveTVGuideSource,
  LiveTVGuideSourcesResponse,
  LiveTVTuner,
  LiveTVTunersResponse,
} from "@/api/types";
import { adminKeys } from "./keys";

const LIVETV_STALE_TIME = 30_000;

export function useLiveTVTuners() {
  return useQuery({
    queryKey: adminKeys.liveTVTuners(),
    queryFn: () => api<LiveTVTunersResponse>("/livetv/tuners").then((data) => data.tuners ?? []),
    staleTime: LIVETV_STALE_TIME,
  });
}

export function useAddLiveTVTuner() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { discover_url?: string; device_id?: string }) =>
      api<LiveTVTuner>("/livetv/tuners", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("HDHomeRun tuner added");
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVTuners() });
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
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
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVTuners() });
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
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
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVTuners() });
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
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
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVChannels() });
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
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
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
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
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
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
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
      toast.success("Guide sync started");
      queryClient.invalidateQueries({ queryKey: adminKeys.liveTVGuideSources() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to sync guide source");
    },
  });
}
