import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  InviteCode,
  CreateInviteCodeRequest,
  UpdateInviteCodeRequest,
  TopUpInviteCodeRequest,
} from "@/api/types";
import { adminKeys } from "../keys";
import { toast } from "sonner";

const ADMIN_STALE_TIME = 30_000;

export function useAdminInviteCodes() {
  return useQuery({
    queryKey: adminKeys.inviteCodes(),
    queryFn: () =>
      api<InviteCode[]>("/admin/invite-codes").then((d) => d ?? []),
    staleTime: ADMIN_STALE_TIME,
  });
}

export function useCreateInviteCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateInviteCodeRequest) =>
      api<InviteCode>("/admin/invite-codes", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Invite code created");
      void queryClient.invalidateQueries({ queryKey: adminKeys.inviteCodes() });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to create invite code",
      );
    },
  });
}

export function useUpdateInviteCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateInviteCodeRequest }) =>
      api(`/admin/invite-codes/${id}`, {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.inviteCodes() });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to update invite code",
      );
    },
  });
}

export function useTopUpInviteCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: TopUpInviteCodeRequest }) =>
      api<InviteCode>(`/admin/invite-codes/${id}/top-up`, {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      toast.success("Invite code topped up");
      void queryClient.invalidateQueries({ queryKey: adminKeys.inviteCodes() });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to top up invite code",
      );
    },
  });
}

export function useDeleteInviteCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      api(`/admin/invite-codes/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Invite code deleted");
      void queryClient.invalidateQueries({ queryKey: adminKeys.inviteCodes() });
    },
    onError: (err) => {
      toast.error(
        err instanceof Error ? err.message : "Failed to delete invite code",
      );
    },
  });
}
