import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { AdminAPIKey, AdminCreateAPIKeyRequest } from "@/api/types";
import { adminKeys } from "../keys";
import { toast } from "sonner";

const ADMIN_STALE_TIME = 30_000;

export function useAdminApiKeys() {
  return useQuery({
    queryKey: adminKeys.apiKeys(),
    queryFn: () => api<AdminAPIKey[]>("/admin/api-keys").then((d) => d ?? []),
    staleTime: ADMIN_STALE_TIME,
  });
}

export function useAdminCreateApiKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: AdminCreateAPIKeyRequest) =>
      api<AdminAPIKey>("/admin/api-keys", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.apiKeys() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to create API key");
    },
  });
}

export function useAdminDeleteApiKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api(`/admin/api-keys/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("API key revoked");
      void queryClient.invalidateQueries({ queryKey: adminKeys.apiKeys() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to revoke API key");
    },
  });
}

export function useAdminUpdateApiKeyTier() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, tier }: { id: number; tier: string }) =>
      api(`/admin/api-keys/${id}/tier`, {
        method: "PUT",
        body: JSON.stringify({ tier }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.apiKeys() });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to update tier");
    },
  });
}
