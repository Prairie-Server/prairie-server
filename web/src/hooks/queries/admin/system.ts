import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { adminKeys } from "../keys";

export interface BuildInfo {
  display: string;
  revision: string;
  dirty: boolean;
  vcs_time: string;
  available: boolean;
  version?: string;
  latest_version?: string;
  update_status?: "up_to_date" | "update_available" | "unknown";
  changelog_url?: string;
  release_url?: string;
}

export interface HWAccelInfo {
  resolved: string;
  render_devices: string[];
  intel_detected: boolean;
  source: "local" | "transcode_node";
  node_url?: string;
}

export function useBuildInfo() {
  return useQuery({
    queryKey: adminKeys.buildInfo(),
    queryFn: () => api<BuildInfo>("/admin/system/build"),
    // Allow periodic refresh so update status can catch newly published releases.
    staleTime: 5 * 60_000,
    retry: false,
  });
}

export function useHWAccelDetection(enabled = true) {
  return useQuery({
    queryKey: adminKeys.hwAccel(),
    queryFn: () => api<HWAccelInfo>("/admin/system/hw-accel"),
    staleTime: 60_000,
    retry: false,
    enabled,
  });
}
