import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  CreateHistoryImportRunRequest,
  EmbyConnectLoginRequest,
  EmbyConnectLoginResponse,
  HistoryImportRun,
  HistoryImportSource,
  PlexCheckRequest,
  PlexCheckResponse,
  PlexPinResponse,
} from "@/api/types";
import { historyImportKeys } from "./keys";
import { toast } from "sonner";

const STALE_TIME = 15_000;

export function useHistoryImportSources() {
  return useQuery({
    queryKey: historyImportKeys.sources(),
    queryFn: () => api<HistoryImportSource[]>("/history-imports/sources").then((d) => d ?? []),
    staleTime: STALE_TIME,
  });
}

export function useHistoryImportRuns(limit = 10) {
  return useQuery({
    queryKey: historyImportKeys.runs(limit),
    queryFn: () =>
      api<HistoryImportRun[]>(`/history-imports/runs?limit=${limit}`).then((d) => d ?? []),
    staleTime: 5_000,
  });
}

export function useHistoryImportRun(id?: string) {
  return useQuery({
    queryKey: historyImportKeys.run(id),
    queryFn: () => api<HistoryImportRun>(`/history-imports/runs/${id}`),
    enabled: !!id,
  });
}

export function useLoginEmbyConnect() {
  return useMutation({
    mutationFn: (body: EmbyConnectLoginRequest) =>
      api<EmbyConnectLoginResponse>("/history-imports/emby-connect/login", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to sign in with Emby Connect");
    },
  });
}

export function useCreatePlexPin() {
  return useMutation({
    mutationFn: () =>
      api<PlexPinResponse>("/history-imports/plex/auth/pin", {
        method: "POST",
      }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to start Plex sign-in");
    },
  });
}

export function useCheckPlexPin(sessionId?: string) {
  return useQuery({
    queryKey: historyImportKeys.plexCheck(sessionId),
    queryFn: () =>
      api<PlexCheckResponse>("/history-imports/plex/auth/check", {
        method: "POST",
        body: JSON.stringify({ session_id: sessionId! } satisfies PlexCheckRequest),
      }),
    enabled: !!sessionId,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return 2_000;
      return data.authenticated ? false : 2_000;
    },
  });
}

export function useCreateHistoryImportRun() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateHistoryImportRunRequest) =>
      api<HistoryImportRun>("/history-imports/runs", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: (run) => {
      toast.success("Import started");
      void queryClient.invalidateQueries({ queryKey: historyImportKeys.runs() });
      void queryClient.invalidateQueries({ queryKey: historyImportKeys.run(run.id) });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to start import");
    },
  });
}
