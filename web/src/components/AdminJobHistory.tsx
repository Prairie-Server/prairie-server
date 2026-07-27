import type { AdminJob } from "@/api/types";
import { useAllAdminJobs } from "@/hooks/queries/admin/libraries";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshCw } from "lucide-react";
import { formatJobProgress } from "./adminCatalogMaintenanceFormatters";
import { formatDateTime } from "@/lib/datetime";
import { stringifyUnknown } from "@/lib/stringifyUnknown";

const JOB_TYPE_LABELS: Record<string, string> = {
  delete_library: "Library Delete",
  image_cache_cleanup: "Image Cache Cleanup",
  catalog_export: "Catalog Export",
  catalog_import: "Catalog Import",
  item_refresh: "Item Refresh",
  library_refresh: "Library Refresh",
};

function jobTypeLabel(jobType: string) {
  return JOB_TYPE_LABELS[jobType] ?? jobType;
}

function statusVariant(status: string) {
  switch (status) {
    case "failed":
      return "destructive" as const;
    case "completed":
      return "secondary" as const;
    case "running":
      return "default" as const;
    default:
      return "outline" as const;
  }
}

function asFiniteNumber(value: unknown, fallback = 0): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const n = Number(value);
    if (Number.isFinite(n)) return n;
  }
  return fallback;
}

function jobDescription(job: AdminJob) {
  const payload = job.request_payload as Record<string, unknown>;
  const libraryName = stringifyUnknown(payload.library_name);
  const libraryId = stringifyUnknown(payload.library_id);
  switch (job.job_type) {
    case "delete_library":
    case "image_cache_cleanup":
      return libraryName ? `"${libraryName}"` : libraryId ? `Library #${libraryId}` : "Library";
    case "item_refresh":
    case "library_refresh":
      return libraryName
        ? `"${libraryName}"`
        : libraryId
          ? `Library #${libraryId}`
          : "All libraries";
    case "catalog_export": {
      const ids = payload.library_ids as number[] | undefined;
      if (ids && ids.length > 0) return `${ids.length} librar${ids.length === 1 ? "y" : "ies"}`;
      return "All libraries";
    }
    case "catalog_import":
      return (
        stringifyUnknown(payload.source_label) ||
        stringifyUnknown(payload.source_key) ||
        "Catalog seed"
      );
    default:
      return "";
  }
}

function jobResult(job: AdminJob) {
  if (job.status !== "completed") return null;
  const result = job.result_payload as Record<string, unknown>;
  switch (job.job_type) {
    case "library_refresh": {
      const total = asFiniteNumber(result.total_items);
      const withIDs = asFiniteNumber(result.items_with_ids);
      const withoutIDs = asFiniteNumber(result.items_without_ids);
      const refreshedOK = asFiniteNumber(result.refreshed_ok);
      const refreshedFailed = asFiniteNumber(result.refreshed_failed);
      const pipelineOK = asFiniteNumber(result.pipeline_ok);
      const pipelineFailed = asFiniteNumber(result.pipeline_failed);
      if (total === 0) {
        return "No library items to refresh";
      }
      return `Total ${total}, ${withIDs} direct, ${withoutIDs} unmatched, direct ${refreshedOK} ok/${refreshedFailed} failed, pipeline ${pipelineOK} ok/${pipelineFailed} failed`;
    }
    case "delete_library": {
      const files = typeof result.deleted_media_files === "number" ? result.deleted_media_files : 0;
      const items =
        typeof result.deleted_orphaned_items === "number" ? result.deleted_orphaned_items : 0;
      const cleanupQueued = result.image_cleanup_queued === true;
      const cleanupDirs =
        typeof result.image_cleanup_dirs === "number" ? result.image_cleanup_dirs : 0;
      const parts = [];
      if (files > 0) parts.push(`${files} files`);
      if (items > 0) parts.push(`${items} items`);
      if (cleanupQueued && cleanupDirs > 0) {
        parts.push(
          `queued cache cleanup for ${cleanupDirs} director${cleanupDirs === 1 ? "y" : "ies"}`,
        );
      }
      return parts.length > 0 ? `Deleted ${parts.join(", ")}` : "Deleted (empty)";
    }
    case "image_cache_cleanup": {
      const deletedPrefixes =
        typeof result.deleted_prefixes === "number" ? result.deleted_prefixes : 0;
      const deletedObjects =
        typeof result.deleted_s3_objects === "number" ? result.deleted_s3_objects : 0;
      return `Deleted ${deletedObjects} cached object${deletedObjects === 1 ? "" : "s"} across ${deletedPrefixes} prefix${deletedPrefixes === 1 ? "" : "es"}`;
    }
    case "catalog_export":
      return typeof result.items_exported === "number"
        ? `Exported ${result.items_exported} items, ${typeof result.files_exported === "number" ? result.files_exported : 0} files`
        : null;
    case "catalog_import":
      return typeof result.items_created === "number"
        ? `Imported ${result.items_created} items, ${typeof result.files_created === "number" ? result.files_created : 0} files`
        : null;
    default:
      return null;
  }
}

export default function AdminJobHistory() {
  const jobsQuery = useAllAdminJobs();
  const jobs = jobsQuery.data ?? [];

  return (
    <div className="border-border/70 bg-card/60 rounded-lg border">
      <div className="border-border/70 flex items-center justify-between border-b px-4 py-3">
        <div>
          <h3 className="text-sm font-semibold">Job History</h3>
          <p className="text-muted-foreground text-xs">Recent background jobs across all types.</p>
        </div>
        <div className="flex items-center gap-2">
          {jobsQuery.isFetching ? (
            <Badge variant="outline">Refreshing</Badge>
          ) : (
            <Badge variant="secondary">{jobs.length}</Badge>
          )}
          <Button
            variant="ghost"
            size="icon"
            aria-label="Refresh job history"
            onClick={() => jobsQuery.refetch()}
          >
            <RefreshCw
              className={`h-4 w-4 ${jobsQuery.isFetching ? "animate-spin" : ""}`}
              aria-hidden="true"
            />
          </Button>
        </div>
      </div>
      <div className="divide-border/60 divide-y">
        {jobs.length === 0 ? (
          <div className="text-muted-foreground px-4 py-5 text-sm">No jobs yet.</div>
        ) : (
          jobs.map((job) => {
            const desc = jobDescription(job);
            const result = jobResult(job);
            const progress = formatJobProgress(job);

            return (
              <div key={job.id} className="flex flex-col gap-1 px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={statusVariant(job.status)}>{job.status}</Badge>
                  <Badge variant="outline">{jobTypeLabel(job.job_type)}</Badge>
                  {desc && <span className="text-sm font-medium">{desc}</span>}
                  <span className="text-muted-foreground text-xs">
                    {formatDateTime(job.requested_at)}
                  </span>
                </div>
                <div className="text-muted-foreground flex flex-wrap gap-3 text-xs">
                  {job.message && <span>{job.message}</span>}
                  {(job.status === "running" || job.status === "queued") && (
                    <span>Progress: {progress}</span>
                  )}
                  {result && <span>{result}</span>}
                  {job.completed_at && <span>Finished: {formatDateTime(job.completed_at)}</span>}
                </div>
                {job.error_message && (
                  <div className="text-destructive text-xs">{job.error_message}</div>
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
