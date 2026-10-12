import { type QueryClient, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { ItemDetail } from "@/api/types";
import { v2 } from "@/api/v2/request";
import { fetchCatalogItemDetail } from "./catalogRead";
import { invalidateRatingSurfaceQueries } from "./ratingsSurfaceRefresh";
import {
  cancelItemDetailQueries,
  isItemDetailQueryKey,
  updateCatalogItemDetail,
} from "./mediaSurfaceRefresh";
import {
  PERSONAL_STATE_WRITE_TIMEOUT_MS,
  personalStateMutationOptions,
} from "./personalStateWrites";

const communityRefreshGeneration = new Map<string, number>();

/**
 * Re-reads the item's community rating after the viewer's own rating changed.
 * The detail query itself stays un-invalidated (#540: refetching it re-renders
 * the whole page), so only `community_rating` is patched in. The server hides
 * the aggregate below its minimum count, so it can't be derived client-side.
 * A newer write for the same item supersedes an older refresh in flight.
 */
export async function refreshCommunityRating(queryClient: QueryClient, itemId: string) {
  const generation = (communityRefreshGeneration.get(itemId) ?? 0) + 1;
  communityRefreshGeneration.set(itemId, generation);
  try {
    const fresh = await fetchCatalogItemDetail(itemId);
    if (communityRefreshGeneration.get(itemId) !== generation) return;
    updateCatalogItemDetail(queryClient, itemId, (detail) => ({
      ...detail,
      community_rating: fresh.community_rating ?? null,
    }));
  } catch {
    // Best effort: the next detail load shows the current figure.
  }
}

export function useSetRating(itemId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    ...personalStateMutationOptions,
    mutationFn: (rating: number) =>
      v2("PUT /api/v2/ratings/{item_id}", {
        path: { item_id: itemId },
        body: { rating },
        timeoutMs: PERSONAL_STATE_WRITE_TIMEOUT_MS,
      }),
    onMutate: async (rating: number) => {
      await cancelItemDetailQueries(queryClient, itemId);
      const previous = queryClient.getQueriesData<ItemDetail>({
        predicate: (query) => isItemDetailQueryKey(query.queryKey, itemId),
      });
      updateCatalogItemDetail(queryClient, itemId, (detail) => ({
        ...detail,
        user_rating: rating,
      }));
      return { previous };
    },
    onError: (_err, _vars, context) => {
      for (const [queryKey, value] of context?.previous ?? []) {
        queryClient.setQueryData(queryKey, value);
      }
      toast.error("Failed to update rating");
    },
    onSuccess: () => {
      void refreshCommunityRating(queryClient, itemId);
    },
    onSettled: () => {
      return invalidateRatingSurfaceQueries(queryClient, itemId);
    },
  });
}

export function useDeleteRating(itemId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    ...personalStateMutationOptions,
    mutationFn: () =>
      v2("DELETE /api/v2/ratings/{item_id}", {
        path: { item_id: itemId },
        timeoutMs: PERSONAL_STATE_WRITE_TIMEOUT_MS,
      }),
    onMutate: async () => {
      await cancelItemDetailQueries(queryClient, itemId);
      const previous = queryClient.getQueriesData<ItemDetail>({
        predicate: (query) => isItemDetailQueryKey(query.queryKey, itemId),
      });
      updateCatalogItemDetail(queryClient, itemId, (detail) => ({
        ...detail,
        user_rating: null,
      }));
      return { previous };
    },
    onError: (_err, _vars, context) => {
      for (const [queryKey, value] of context?.previous ?? []) {
        queryClient.setQueryData(queryKey, value);
      }
      toast.error("Failed to update rating");
    },
    onSuccess: () => {
      void refreshCommunityRating(queryClient, itemId);
    },
    onSettled: () => {
      return invalidateRatingSurfaceQueries(queryClient, itemId);
    },
  });
}
