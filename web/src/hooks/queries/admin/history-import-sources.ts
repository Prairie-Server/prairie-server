import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  CreateHistoryImportSourceRequest,
  HistoryImportExternalUser,
  HistoryImportSource,
  SetHistoryImportAdminTokenRequest,
  UpdateHistoryImportSourceRequest,
} from "@/api/types";
import { adminKeys, historyImportKeys } from "../keys";
import { toast } from "sonner";

export function useAdminHistoryImportSources() {
  return useQuery({
    queryKey: adminKeys.historyImportSources(),
    queryFn: () =>
      api<HistoryImportSource[]>("/admin/history-import-sources").then(
        (d) => d ?? [],
      ),
  });
}

export function useCreateAdminHistoryImportSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateHistoryImportSourceRequest) =>
      api<HistoryImportSource>("/admin/history-import-sources", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Saved server created");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.historyImportSources(),
      });
      void queryClient.invalidateQueries({
        queryKey: historyImportKeys.sources(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to create saved server",
      );
    },
  });
}

export function useUpdateAdminHistoryImportSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: number;
      body: UpdateHistoryImportSourceRequest;
    }) =>
      api<HistoryImportSource>(`/admin/history-import-sources/${id}`, {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Saved server updated");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.historyImportSources(),
      });
      void queryClient.invalidateQueries({
        queryKey: historyImportKeys.sources(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to update saved server",
      );
    },
  });
}

export function useDeleteAdminHistoryImportSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      api(`/admin/history-import-sources/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Saved server deleted");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.historyImportSources(),
      });
      void queryClient.invalidateQueries({
        queryKey: historyImportKeys.sources(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to delete saved server",
      );
    },
  });
}

export function useSetAdminSourceToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: number;
      body: SetHistoryImportAdminTokenRequest;
    }) =>
      api(`/admin/history-imports/sources/${id}/token`, {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Admin token saved");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.historyImportSources(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to save admin token",
      );
    },
  });
}

export function useClearAdminSourceToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      api(`/admin/history-imports/sources/${id}/token`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Admin token removed");
      void queryClient.invalidateQueries({
        queryKey: adminKeys.historyImportSources(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to remove admin token",
      );
    },
  });
}

export function useDiscoverExternalUsers(sourceId: number | undefined) {
  return useQuery({
    queryKey: adminKeys.historyImportExternalUsers(sourceId ?? 0),
    queryFn: () =>
      api<HistoryImportExternalUser[]>(
        `/admin/history-imports/sources/${sourceId}/users`,
      ).then((d) => d ?? []),
    enabled: false, // manually triggered
    retry: false,
  });
}

export function usePlexLogin() {
  return useMutation({
    mutationFn: (body: { username: string; password: string }) =>
      api<{ token: string }>("/admin/history-imports/plex/login", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Plex login failed");
    },
  });
}
