import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { AdminPlaybackHistoryItem, AdminUserProfile } from "@/api/types";
import { adminKeys } from "../keys";

const ADMIN_HISTORY_STALE_TIME = 15_000;

export interface AdminPlaybackHistoryParams {
  userId?: number;
  profileId?: string;
  mediaItemId?: string;
  completed?: "all" | "true" | "false";
  limit?: number;
}

export function buildAdminPlaybackHistorySearchParams(
  params: AdminPlaybackHistoryParams,
) {
  const search = new URLSearchParams();
  if (params.userId) search.set("user_id", String(params.userId));
  if (params.profileId) search.set("profile_id", params.profileId);
  if (params.mediaItemId) search.set("media_item_id", params.mediaItemId);
  if (params.completed && params.completed !== "all") {
    search.set("completed", params.completed);
  } else {
    search.set("completed", "all");
  }
  search.set("limit", String(params.limit ?? 100));
  return search;
}

export function useAdminPlaybackHistory(params: AdminPlaybackHistoryParams) {
  return useQuery({
    queryKey: adminKeys.playbackHistory(params),
    queryFn: () =>
      api<AdminPlaybackHistoryItem[]>(
        `/admin/playback-history?${buildAdminPlaybackHistorySearchParams(params).toString()}`,
      ).then((rows) => rows ?? []),
    staleTime: ADMIN_HISTORY_STALE_TIME,
    refetchInterval: ADMIN_HISTORY_STALE_TIME,
    refetchIntervalInBackground: true,
  });
}

export function useAdminUserProfiles(userId?: number) {
  return useQuery({
    queryKey: adminKeys.userProfiles(userId),
    queryFn: () =>
      api<AdminUserProfile[]>(`/admin/users/${userId}/profiles`).then(
        (rows) => rows ?? [],
      ),
    enabled: Boolean(userId),
    staleTime: ADMIN_HISTORY_STALE_TIME,
  });
}
