import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { api } from "@/api/client";
import type {
  ImportUserCollectionResponse,
  ImportUserMDBListCollectionRequest,
  ImportUserTMDBCollectionRequest,
  ImportUserTraktCollectionRequest,
  MDBListDiscoveryResponse,
  UserCollectionSyncResult,
} from "@/api/types";
import {
  TEMPLATE_STALE_TIME,
  type CollectionTemplateCatalog,
} from "@/lib/collectionTemplates";
import { invalidateUserCollectionQueries } from "./collectionSurfaceRefresh";
import { collectionKeys } from "./keys";

export function useUserCollectionTemplates(enabled = true) {
  return useQuery({
    queryKey: collectionKeys.templates(),
    queryFn: () => api<CollectionTemplateCatalog>("/collections/templates"),
    enabled,
    staleTime: TEMPLATE_STALE_TIME,
  });
}

export function useMDBListSearch(query: string, enabled = true) {
  const trimmed = query.trim();
  return useQuery({
    queryKey: collectionKeys.mdblistSearch(trimmed),
    queryFn: () =>
      api<MDBListDiscoveryResponse>(
        `/collections/import/mdblist/search?q=${encodeURIComponent(trimmed)}`,
      ),
    enabled: enabled && trimmed.length > 0,
    staleTime: 60_000,
  });
}

export function useMDBListTop(enabled = true) {
  return useQuery({
    queryKey: collectionKeys.mdblistTop(),
    queryFn: () =>
      api<MDBListDiscoveryResponse>("/collections/import/mdblist/top"),
    enabled,
    staleTime: 5 * 60_000,
  });
}

function importToastMessage(label: string, status: string | undefined) {
  if (status === "warning") return `${label} imported with warnings`;
  if (status === "failed") return `${label} imported but sync failed`;
  return `${label} imported`;
}

export function useImportUserMDBListCollection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: ImportUserMDBListCollectionRequest) =>
      api<ImportUserCollectionResponse>("/collections/import/mdblist", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: (result) => {
      toast.success(importToastMessage("MDBList", result.sync?.status));
      void invalidateUserCollectionQueries(queryClient);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : "Import failed");
    },
  });
}

export function useImportUserTMDBCollection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: ImportUserTMDBCollectionRequest) =>
      api<ImportUserCollectionResponse>("/collections/import/tmdb", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: (result) => {
      toast.success(importToastMessage("TMDB collection", result.sync?.status));
      void invalidateUserCollectionQueries(queryClient);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : "Import failed");
    },
  });
}

export function useImportUserTraktCollection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: ImportUserTraktCollectionRequest) =>
      api<ImportUserCollectionResponse>("/collections/import/trakt", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: (result) => {
      toast.success(
        importToastMessage("Trakt collection", result.sync?.status),
      );
      void invalidateUserCollectionQueries(queryClient);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : "Import failed");
    },
  });
}

export function useSyncUserCollection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (collectionId: string) =>
      api<UserCollectionSyncResult>(`/collections/${collectionId}/sync`, {
        method: "POST",
      }),
    onSuccess: (result, collectionId) => {
      const matched = `${result.items_matched} item${result.items_matched === 1 ? "" : "s"}`;
      const message =
        result.status === "warning"
          ? `Synced with warnings — matched ${matched}`
          : `Synced — matched ${matched}`;
      toast.success(message);
      void invalidateUserCollectionQueries(queryClient, collectionId);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : "Sync failed");
    },
  });
}
