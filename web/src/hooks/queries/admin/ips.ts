import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { UserIPEntry, IPUserEntry } from "@/api/types";
import { adminKeys } from "../keys";

const ADMIN_STALE_TIME = 30_000;

export function useUserIPs(userId: number, days = 30) {
  return useQuery({
    queryKey: adminKeys.userIPs(userId, days),
    queryFn: () =>
      api<UserIPEntry[]>(`/admin/users/${userId}/ips?days=${days}`).then(
        (d) => d ?? [],
      ),
    staleTime: ADMIN_STALE_TIME,
  });
}

export function useIPUsers(ip: string, days = 30) {
  return useQuery({
    queryKey: adminKeys.ipUsers(ip, days),
    queryFn: () =>
      api<IPUserEntry[]>(
        `/admin/ips?ip=${encodeURIComponent(ip)}&days=${days}`,
      ).then((d) => d ?? []),
    staleTime: ADMIN_STALE_TIME,
    enabled: ip.length > 0,
  });
}
