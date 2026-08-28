import type { ReactNode } from "react";
import { ExternalLink } from "lucide-react";

import { useBuildInfo, type BuildInfo } from "@/hooks/queries/admin/system";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";

function updateStatusLabel(status: BuildInfo["update_status"]): string {
  switch (status) {
    case "up_to_date":
      return "Up to date";
    case "update_available":
      return "Update available";
    case "unknown":
      return "Couldn't compare versions";
    default:
      return "Unknown";
  }
}

function updateStatusBadgeClass(status: BuildInfo["update_status"]): string {
  if (status === "update_available") {
    return "border-amber-500/40 text-amber-500";
  }
  if (status === "up_to_date") {
    return "border-emerald-500/40 text-emerald-600 dark:text-emerald-400";
  }
  return "";
}

function InfoRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 border-b py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <div className="text-muted-foreground text-sm">{label}</div>
      <div className="text-sm font-medium break-all sm:text-right">{value}</div>
    </div>
  );
}

export default function AboutSettings() {
  const buildInfo = useBuildInfo();

  if (buildInfo.isPending && !buildInfo.data) {
    return (
      <div className="space-y-6" role="status" aria-label="Loading about">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-4 w-72" />
        <Skeleton className="h-40 w-full rounded-xl" />
        <span className="sr-only">Loading about</span>
      </div>
    );
  }

  if (buildInfo.isError || !buildInfo.data) {
    return (
      <div className="space-y-2">
        <h2 className="text-xl font-semibold tracking-tight">About</h2>
        <p className="text-muted-foreground text-sm">
          Could not load server build information.
        </p>
      </div>
    );
  }

  const info = buildInfo.data;
  const version = info.version || info.display || "dev build";
  const changelogURL =
    info.changelog_url ||
    "https://github.com/Prairie-Server/prairie-server/releases";

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h2 className="text-xl font-semibold tracking-tight">About</h2>
        <p className="text-muted-foreground text-sm leading-relaxed">
          Current Prairie Server version, update status, and release notes.
        </p>
      </div>

      <section className="surface-panel-subtle rounded-[1.25rem] px-5 py-2">
        <InfoRow
          label="Version"
          value={<span className="font-mono">{version}</span>}
        />
        {info.revision ? (
          <InfoRow
            label="Revision"
            value={
              <span className="font-mono">
                {info.display !== version ? `${info.display} · ` : ""}
                {info.revision.slice(0, 12)}
                {info.dirty ? " (dirty)" : ""}
              </span>
            }
          />
        ) : null}
        <InfoRow
          label="Update status"
          value={
            <Badge
              variant="outline"
              className={updateStatusBadgeClass(info.update_status)}
            >
              {updateStatusLabel(info.update_status)}
            </Badge>
          }
        />
        {info.latest_version ? (
          <InfoRow
            label="Latest version"
            value={<span className="font-mono">{info.latest_version}</span>}
          />
        ) : null}
        <InfoRow
          label="Changelog"
          value={
            <a
              href={changelogURL}
              target="_blank"
              rel="noreferrer"
              className="text-primary inline-flex items-center gap-1.5 hover:underline"
            >
              View release notes
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          }
        />
        {info.release_url && info.release_url !== changelogURL ? (
          <InfoRow
            label="Latest release"
            value={
              <a
                href={info.release_url}
                target="_blank"
                rel="noreferrer"
                className="text-primary inline-flex items-center gap-1.5 hover:underline"
              >
                Open
                <ExternalLink className="h-3.5 w-3.5" />
              </a>
            }
          />
        ) : null}
      </section>
    </div>
  );
}
