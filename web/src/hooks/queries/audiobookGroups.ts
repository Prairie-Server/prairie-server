import { useInfiniteQuery } from "@tanstack/react-query";

import { api } from "@/api/client";
import type { AudiobookGroupsResponse } from "@/api/types";
import { catalogKeys } from "./keys";

export type AudiobookGroupBy = "author" | "narrator" | "series";
export type AudiobookGroupSort = "name" | "count" | "duration";

const GROUPS_PAGE_SIZE = 60;

export async function fetchAudiobookGroupsPage(
  libraryId: number,
  groupBy: AudiobookGroupBy,
  sort: AudiobookGroupSort,
  offset: number,
  includeTotal: boolean,
  searchPrefix: string,
  options?: RequestInit,
): Promise<AudiobookGroupsResponse> {
  const params = new URLSearchParams({
    library_id: String(libraryId),
    group_by: groupBy,
    sort,
    limit: String(GROUPS_PAGE_SIZE),
    offset: String(offset),
    include_total: String(includeTotal),
  });
  const trimmedSearch = searchPrefix.trim();
  if (trimmedSearch) {
    params.set("q", trimmedSearch);
  }

  return api<AudiobookGroupsResponse>(
    `/catalog/audiobook-groups?${params.toString()}`,
    options,
  );
}

export function useAudiobookGroups(
  libraryId: number,
  groupBy: AudiobookGroupBy,
  sort: AudiobookGroupSort,
  searchPrefix = "",
) {
  const search = searchPrefix.trim();
  return useInfiniteQuery({
    queryKey: catalogKeys.audiobookGroups(libraryId, groupBy, sort, search),
    queryFn: ({
      pageParam,
      signal,
    }: {
      pageParam: number;
      signal: AbortSignal;
    }) =>
      fetchAudiobookGroupsPage(
        libraryId,
        groupBy,
        sort,
        pageParam,
        pageParam === 0,
        search,
        {
          signal,
        },
      ),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      if (!lastPage.has_more) {
        return undefined;
      }
      return allPages.reduce((offset, page) => offset + page.groups.length, 0);
    },
    enabled: libraryId > 0,
    staleTime: 60_000,
  });
}
