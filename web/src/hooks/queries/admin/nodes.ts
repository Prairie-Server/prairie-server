import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  StreamNode,
  CreateNodeRequest,
  CheckNodeResponse,
} from "@/api/types";
import { adminKeys } from "../keys";
import { toast } from "sonner";

const ADMIN_STALE_TIME = 30_000;

export function useAdminNodes() {
  return useQuery({
    queryKey: adminKeys.nodes(),
    queryFn: () => api<StreamNode[]>("/admin/nodes").then((d) => d ?? []),
    staleTime: ADMIN_STALE_TIME,
  });
}

export function useCreateNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateNodeRequest) =>
      api("/admin/nodes", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Node created");
      void queryClient.invalidateQueries({ queryKey: adminKeys.nodes() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to save");
    },
  });
}

export function useUpdateNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: Record<string, unknown> }) =>
      api<StreamNode>(`/admin/nodes/${id}`, {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.nodes() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update node");
    },
  });
}

export function useDeleteNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api(`/admin/nodes/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Node deleted");
      void queryClient.invalidateQueries({ queryKey: adminKeys.nodes() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to delete node");
    },
  });
}

export function useCheckNodeHealth() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (node: StreamNode) =>
      api<CheckNodeResponse>(`/admin/nodes/${node.id}/check`, {
        method: "POST",
      }).then((result) => ({ node, result })),
    onSuccess: ({ node, result }) => {
      toast.success(
        result.healthy
          ? `${node.name} is healthy`
          : `${node.name} is unhealthy`,
      );
      void queryClient.invalidateQueries({ queryKey: adminKeys.nodes() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Health check failed");
    },
  });
}

export function useToggleNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (node: StreamNode) =>
      api<StreamNode>(`/admin/nodes/${node.id}`, {
        method: "PUT",
        body: JSON.stringify({ enabled: !node.enabled }),
      }),
    onSuccess: (updated) => {
      toast.success(
        `${updated.name} ${updated.enabled ? "enabled" : "disabled"}`,
      );
      void queryClient.invalidateQueries({ queryKey: adminKeys.nodes() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update node");
    },
  });
}
