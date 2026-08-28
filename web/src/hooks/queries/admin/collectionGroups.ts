import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import type {
  GroupSortMode,
  LibraryCollection,
  LibraryCollectionGroup,
} from "@/api/types";
import { adminKeys } from "../keys";

const ADMIN_STALE_TIME = 30_000;

// ----- Queries -----

interface GroupListResponse {
  groups: LibraryCollectionGroup[];
  ungrouped_sort_order: number;
}

export function useCollectionGroups(libraryId: number | undefined) {
  return useQuery({
    queryKey:
      libraryId !== undefined
        ? adminKeys.collectionGroups(libraryId)
        : ["admin", "collection-groups-board", "none"],
    queryFn: async () => {
      const res = await api<GroupListResponse>(
        `/admin/libraries/${libraryId}/collection-groups`,
      );
      return res.groups;
    },
    staleTime: ADMIN_STALE_TIME,
    enabled: typeof libraryId === "number" && libraryId > 0,
  });
}

/**
 * Combined board: groups + their collections (resolved client-side from
 * /admin/collections?library_id=N filtered by group_id) + the ungrouped bucket.
 * The backend currently doesn't return collections nested under groups in the
 * admin list; we fetch both and merge here.
 */
export function useAdminCollectionsBoard(libraryId: number | undefined) {
  return useQuery({
    queryKey:
      libraryId !== undefined
        ? [...adminKeys.collectionGroups(libraryId), "with-collections"]
        : ["admin", "collection-groups-board", "none"],
    queryFn: async () => {
      const [groupsResp, collectionsResp] = await Promise.all([
        api<GroupListResponse>(
          `/admin/libraries/${libraryId}/collection-groups`,
        ),
        api<{ collections: LibraryCollection[] }>(
          `/admin/collections?library_id=${libraryId}`,
        ),
      ]);
      const collections = collectionsResp.collections;
      const groups = groupsResp.groups
        .slice()
        .sort(
          (a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name),
        )
        .map((g) => ({
          ...g,
          collections: collections
            .filter((c) => c.group_id === g.id)
            .sort(
              (a, b) =>
                a.sort_order - b.sort_order || a.title.localeCompare(b.title),
            ),
        }));
      const ungrouped = collections
        .filter((c) => !c.group_id)
        .sort(
          (a, b) =>
            a.sort_order - b.sort_order || a.title.localeCompare(b.title),
        );
      const ungroupedSortOrder: number =
        groupsResp.ungrouped_sort_order ?? 9999;
      return { groups, ungrouped, ungroupedSortOrder };
    },
    staleTime: ADMIN_STALE_TIME,
    enabled: typeof libraryId === "number" && libraryId > 0,
  });
}

// ----- Mutations -----

interface CreateGroupInput {
  name: string;
  slug?: string;
  default_sort_mode?: GroupSortMode;
}

export function useCreateCollectionGroup(libraryId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateGroupInput) =>
      api<LibraryCollectionGroup>(
        `/admin/libraries/${libraryId}/collection-groups`,
        {
          method: "POST",
          body: JSON.stringify(input),
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.collectionGroups(libraryId),
      });
      toast.success("Group created");
    },
    onError: (e) =>
      toast.error(
        `Failed to create group: ${e instanceof Error ? e.message : "unknown"}`,
      ),
  });
}

interface UpdateGroupInput {
  id: string;
  name?: string;
  slug?: string;
  default_sort_mode?: GroupSortMode;
}

export function useUpdateCollectionGroup(libraryId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...patch }: UpdateGroupInput) =>
      api<LibraryCollectionGroup>(`/admin/collection-groups/${id}`, {
        method: "PUT",
        body: JSON.stringify(patch),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.collectionGroups(libraryId),
      });
    },
    onError: (e) =>
      toast.error(
        `Failed to update group: ${e instanceof Error ? e.message : "unknown"}`,
      ),
  });
}

export function useDeleteCollectionGroup(libraryId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<void>(`/admin/collection-groups/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.collectionGroups(libraryId),
      });
      toast.success("Group deleted");
    },
    onError: (e) =>
      toast.error(
        `Failed to delete group: ${e instanceof Error ? e.message : "unknown"}`,
      ),
  });
}

export function useReorderCollectionGroups(libraryId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (orderedIDs: string[]) =>
      api<void>(`/admin/libraries/${libraryId}/collection-groups/reorder`, {
        method: "PUT",
        body: JSON.stringify({ ids: orderedIDs }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.collectionGroups(libraryId),
      });
    },
    onError: (e) =>
      toast.error(
        `Failed to reorder groups: ${e instanceof Error ? e.message : "unknown"}`,
      ),
  });
}

interface ReorderCollectionsInput {
  groupID: string; // pass "ungrouped" for the NULL bucket
  orderedIDs: string[];
  moveOmitted?: "ungrouped";
  libraryId?: number; // required when groupID === "ungrouped"
}

export function useReorderCollectionsInGroup(libraryId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      groupID,
      orderedIDs,
      moveOmitted,
      libraryId: libIDArg,
    }: ReorderCollectionsInput) => {
      const params = new URLSearchParams();
      if (moveOmitted) params.set("move_omitted", moveOmitted);
      if (groupID === "ungrouped") {
        const lid = libIDArg ?? libraryId;
        params.set("library_id", String(lid));
      }
      const qs = params.toString() ? `?${params.toString()}` : "";
      return api<void>(
        `/admin/collection-groups/${groupID}/collections/reorder${qs}`,
        {
          method: "PUT",
          body: JSON.stringify({ ids: orderedIDs }),
        },
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.collectionGroups(libraryId),
      });
    },
    onError: (e) =>
      toast.error(
        `Failed to reorder collections: ${e instanceof Error ? e.message : "unknown"}`,
      ),
  });
}
