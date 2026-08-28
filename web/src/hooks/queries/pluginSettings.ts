import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "@/api/client";
import type {
  PluginSettingsDetailResponse,
  PluginSettingsListResponse,
  UpdatePluginSettingsRequest,
} from "@/api/types";
import { settingsKeys } from "./keys";

export function usePluginSettingsList() {
  return useQuery({
    queryKey: settingsKeys.plugins(),
    queryFn: () =>
      api<PluginSettingsListResponse>("/settings/plugins").then(
        (data) => data ?? { installations: [] },
      ),
    staleTime: 30_000,
  });
}

export function usePluginSettingsDetail(installationId: number, enabled = true) {
  return useQuery({
    queryKey: settingsKeys.pluginDetail(installationId),
    queryFn: () => api<PluginSettingsDetailResponse>(`/settings/plugins/${installationId}`),
    enabled,
    staleTime: 30_000,
  });
}

export function useUpdatePluginSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdatePluginSettingsRequest }) =>
      api(`/settings/plugins/${id}`, {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: (_, { id }) => {
      void queryClient.invalidateQueries({ queryKey: settingsKeys.plugins() });
      void queryClient.invalidateQueries({ queryKey: settingsKeys.pluginDetail(id) });
    },
  });
}
