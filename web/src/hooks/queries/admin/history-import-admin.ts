import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  AdminHistoryImportBulkRunResult,
  CreateHistoryImportMappingRequest,
  HistoryImportRun,
  HistoryImportUserMapping,
  UpdateHistoryImportMappingRequest,
} from "@/api/types";
import { adminKeys } from "../keys";
import { toast } from "sonner";

// --- Mappings ---

export function useAdminHistoryImportMappings(
  sourceId: number | undefined,
  hasActiveRuns?: boolean,
) {
  return useQuery({
    queryKey: adminKeys.historyImportMappings(sourceId),
    queryFn: () =>
      api<HistoryImportUserMapping[]>(`/admin/history-imports/mappings?source_id=${sourceId}`).then(
        (d) => d ?? [],
      ),
    enabled: sourceId != null && sourceId > 0,
    staleTime: hasActiveRuns ? 5_000 : 30_000,
  });
}

export function useCreateAdminMapping() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateHistoryImportMappingRequest) =>
      api<HistoryImportUserMapping>("/admin/history-imports/mappings", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: (_data, variables) => {
      toast.success("User mapping created");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.historyImportMappings(variables.source_id),
      });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to create mapping");
    },
  });
}

export function useUpdateAdminMapping() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateHistoryImportMappingRequest }) =>
      api<HistoryImportUserMapping>(`/admin/history-imports/mappings/${id}`, {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Mapping updated");
      void queryClient.invalidateQueries({ queryKey: ["admin", "historyImportMappings"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update mapping");
    },
  });
}

export function useDeleteAdminMapping() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api(`/admin/history-imports/mappings/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Mapping deleted");
      void queryClient.invalidateQueries({ queryKey: ["admin", "historyImportMappings"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to delete mapping");
    },
  });
}

// --- Runs ---

export function useCreateAdminRunForMapping() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (mappingId: number) =>
      api<HistoryImportRun>(`/admin/history-imports/mappings/${mappingId}/run`, {
        method: "POST",
      }),
    onSuccess: () => {
      toast.success("Import started");
      void queryClient.invalidateQueries({ queryKey: ["admin", "historyImportAdminRuns"] });
      void queryClient.invalidateQueries({ queryKey: ["admin", "historyImportMappings"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to start import");
    },
  });
}

export function useAdminBulkRun() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (sourceId: number) =>
      api<AdminHistoryImportBulkRunResult>(`/admin/history-imports/sources/${sourceId}/bulk-run`, {
        method: "POST",
      }),
    onSuccess: (data) => {
      const count = data?.runs?.length ?? 0;
      toast.success(`Started ${count} import${count !== 1 ? "s" : ""}`);
      void queryClient.invalidateQueries({ queryKey: ["admin", "historyImportAdminRuns"] });
      void queryClient.invalidateQueries({ queryKey: ["admin", "historyImportMappings"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to start bulk import");
    },
  });
}

export function useAdminHistoryImportRuns(sourceId?: number) {
  const params = sourceId != null ? { source_id: sourceId } : {};
  const url =
    sourceId != null
      ? `/admin/history-imports/runs?source_id=${sourceId}`
      : `/admin/history-imports/runs`;
  return useQuery({
    queryKey: adminKeys.historyImportAdminRuns(params),
    queryFn: () => api<HistoryImportRun[]>(url).then((d) => d ?? []),
    staleTime: 5_000,
  });
}

export function useAdminHistoryImportRun(id: string | undefined) {
  return useQuery({
    queryKey: adminKeys.historyImportAdminRun(id),
    queryFn: () => api<HistoryImportRun>(`/admin/history-imports/runs/${id}`),
    enabled: id != null,
  });
}

export function useCancelAdminRun() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (runId: string) =>
      api(`/admin/history-imports/runs/${runId}/cancel`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Run cancelled");
      void queryClient.invalidateQueries({ queryKey: ["admin", "historyImportAdminRuns"] });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to cancel run");
    },
  });
}
