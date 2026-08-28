import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import { adminKeys } from "../keys";
import { toast } from "sonner";

interface JobStatus {
  running: boolean;
  count: number;
  total?: number;
}

interface RecommendationsStatusResponse {
  embeddings: JobStatus;
  taste_profiles: JobStatus;
  cowatch: JobStatus;
  recommendations: JobStatus;
}

export function useRecommendationsStatus() {
  return useQuery({
    queryKey: adminKeys.recommendationsStatus(),
    queryFn: () =>
      api<RecommendationsStatusResponse>("/admin/recommendations/status"),
    refetchInterval: 5000,
  });
}

export function useTriggerEmbeddings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api("/admin/recommendations/trigger/embeddings", { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.recommendationsStatus(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to trigger embeddings",
      );
    },
  });
}

export function useTriggerTasteProfiles() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api("/admin/recommendations/trigger/taste-profiles", { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.recommendationsStatus(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to trigger taste profiles",
      );
    },
  });
}

export function useTriggerCowatch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api("/admin/recommendations/trigger/cowatch", { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.recommendationsStatus(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error
          ? err.message
          : "Failed to trigger co-watch computation",
      );
    },
  });
}

export function useTriggerRecommendations() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api("/admin/recommendations/trigger/recommendations", { method: "POST" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: adminKeys.recommendationsStatus(),
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error
          ? err.message
          : "Failed to trigger recommendations",
      );
    },
  });
}
