import { useEffect } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { v2, type V2Body, type V2OptionalBody, type V2Query } from "@/api/v2/request";
import type { components } from "@/api/v2/schema";
import { adminKeys } from "./keys";

const LIVETV_STALE_TIME = 30_000;

/** Well inside the server's stale-session TTL so a paused player keeps its tuner. */
export const LIVETV_HEARTBEAT_INTERVAL_MS = 30_000;

export function useLiveTVTuners() {
  return useQuery({
    queryKey: adminKeys.liveTVTuners(),
    queryFn: () => v2("GET /api/v2/admin/livetv/tuners").then((data) => data.items ?? []),
    staleTime: LIVETV_STALE_TIME,
  });
}

export function useDiscoverLiveTVTuners() {
  return useMutation({
    mutationFn: (body: V2OptionalBody<"POST /api/v2/admin/livetv/tuners/discover">) =>
      v2("POST /api/v2/admin/livetv/tuners/discover", { body }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Tuner discovery failed");
    },
  });
}

export function useAddLiveTVTuner() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: V2Body<"POST /api/v2/admin/livetv/tuners">) =>
      v2("POST /api/v2/admin/livetv/tuners", { body }),
    onSuccess: () => {
      toast.success("Tuner added");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVTuners(),
      });
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVChannels(),
      });
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
      v2("POST /api/v2/admin/livetv/tuners/{tuner_id}/scan", { path: { tuner_id: tunerId } }),
    onSuccess: () => {
      toast.success("Channel lineup rescanned");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVTuners(),
      });
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVChannels(),
      });
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
      v2("DELETE /api/v2/admin/livetv/tuners/{tuner_id}", { path: { tuner_id: tunerId } }),
    onSuccess: () => {
      toast.success("Tuner removed");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVTuners(),
      });
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVChannels(),
      });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to delete tuner");
    },
  });
}

export function useLiveTVChannels(tunerId?: string) {
  return useQuery({
    queryKey: adminKeys.liveTVChannels(tunerId),
    queryFn: () =>
      v2("GET /api/v2/livetv/channels", { query: tunerId ? { tuner_id: tunerId } : {} }).then(
        (data) => data.items ?? [],
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
      body: V2Body<"PATCH /api/v2/admin/livetv/channels/{channel_id}">;
    }) =>
      v2("PATCH /api/v2/admin/livetv/channels/{channel_id}", {
        path: { channel_id: channelId },
        body,
      }),
    onSuccess: () => {
      toast.success("Channel updated");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVChannels(),
      });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update channel");
    },
  });
}

export function useLiveTVGuideSources() {
  return useQuery({
    queryKey: adminKeys.liveTVGuideSources(),
    queryFn: () => v2("GET /api/v2/admin/livetv/guide-sources").then((data) => data.items ?? []),
    staleTime: LIVETV_STALE_TIME,
  });
}

export function useCreateLiveTVGuideSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: V2Body<"POST /api/v2/admin/livetv/guide-sources">) =>
      v2("POST /api/v2/admin/livetv/guide-sources", { body }),
    onSuccess: () => {
      toast.success("Guide source added");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVGuideSources(),
      });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to add guide source");
    },
  });
}

export function useUpdateLiveTVGuideSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: V2Body<"PATCH /api/v2/admin/livetv/guide-sources/{source_id}">;
    }) =>
      v2("PATCH /api/v2/admin/livetv/guide-sources/{source_id}", {
        path: { source_id: id },
        body,
      }),
    onSuccess: () => {
      toast.success("Guide source updated");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVGuideSources(),
      });
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
      v2("DELETE /api/v2/admin/livetv/guide-sources/{source_id}", { path: { source_id: id } }),
    onSuccess: () => {
      toast.success("Guide source removed");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVGuideSources(),
      });
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
      v2("POST /api/v2/admin/livetv/guide-sources/{source_id}/sync", { path: { source_id: id } }),
    onSuccess: () => {
      toast.success("Guide sync finished");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVGuideSources(),
      });
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVChannels(),
      });
      void queryClient.invalidateQueries({ queryKey: ["livetv", "guide"] });
    },
    onError: (err) => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.liveTVGuideSources(),
      });
      toast.error(err instanceof Error ? err.message : "Failed to sync guide source");
    },
  });
}

export function useLookupSchedulesDirectLineups() {
  return useMutation({
    mutationFn: (body: V2Body<"POST /api/v2/admin/livetv/schedules-direct/lineups">) =>
      v2("POST /api/v2/admin/livetv/schedules-direct/lineups", { body }).then(
        (data) => data.items ?? [],
      ),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to look up lineups");
    },
  });
}

export function useLookupXMLSyncLineups() {
  return useMutation({
    mutationFn: (body: V2Body<"POST /api/v2/admin/livetv/xml-sync/lineups">) =>
      v2("POST /api/v2/admin/livetv/xml-sync/lineups", { body }).then((data) => data.items ?? []),
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
  return useQuery({
    queryKey: adminKeys.liveTVGuide({
      channels: params.channelIds?.join(",") ?? "",
      start: params.start ?? "",
      end: params.end ?? "",
    }),
    queryFn: () =>
      v2("GET /api/v2/livetv/guide", {
        query: {
          channel_id: params.channelIds?.length ? params.channelIds : undefined,
          start: params.start || undefined,
          end: params.end || undefined,
        },
      }).then((data) => ({
        programs: data.items ?? [],
        start: data.start,
        end: data.end,
      })),
    staleTime: LIVETV_STALE_TIME,
    enabled,
    placeholderData: keepPreviousData,
  });
}

/**
 * Codecs this browser can decode, so the server knows whether it can hand us
 * the broadcast streams untouched. OTA channels are MPEG-2 video with AC-3
 * audio, neither of which Media Source Extensions decode — without this the
 * bridge copies them and playback is a black screen with no sound.
 */
export type LiveTVClientCapabilities = components["schemas"]["LiveTVClientCapabilities"];

export function useStartLiveTVSession() {
  return useMutation({
    mutationFn: ({
      channelId,
      capabilities,
    }: {
      channelId: string;
      capabilities?: LiveTVClientCapabilities;
    }) =>
      v2("POST /api/v2/livetv/channels/{channel_id}/sessions", {
        path: { channel_id: channelId },
        body: capabilities ? { client_capabilities: capabilities } : {},
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
      void v2("POST /api/v2/livetv/sessions/{session_id}/heartbeat", {
        path: { session_id: sessionId },
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
      v2("DELETE /api/v2/livetv/sessions/{session_id}", { path: { session_id: sessionId } }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to release Live TV session");
    },
  });
}

export function useLiveTVRecordings(
  status?: NonNullable<V2Query<"GET /api/v2/livetv/recordings">>["status"],
) {
  return useQuery({
    queryKey: adminKeys.liveTVRecordings(status),
    queryFn: () =>
      v2("GET /api/v2/livetv/recordings", { query: status ? { status } : {} }).then(
        (data) => data.items ?? [],
      ),
    staleTime: LIVETV_STALE_TIME,
  });
}

export function useScheduleLiveTVRecording() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: V2Body<"POST /api/v2/livetv/recordings">) =>
      v2("POST /api/v2/livetv/recordings", { body }),
    onSuccess: () => {
      toast.success("Recording scheduled");
      void queryClient.invalidateQueries({
        queryKey: ["livetv", "recordings"],
      });
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
      v2("DELETE /api/v2/livetv/recordings/{recording_id}", {
        path: { recording_id: recordingId },
      }),
    onSuccess: () => {
      toast.success("Recording cancelled");
      void queryClient.invalidateQueries({
        queryKey: ["livetv", "recordings"],
      });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to cancel recording");
    },
  });
}
