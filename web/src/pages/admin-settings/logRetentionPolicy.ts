import { stringifyUnknown } from "@/lib/stringifyUnknown";
export const OPSLOG_RETENTION_DAYS_KEY = "opslog.retention_days";
export const OPSLOG_CLEANUP_INTERVAL_KEY = "opslog.cleanup_interval_minutes";
export const OPSLOG_MAX_ROWS_KEY = "opslog.max_rows";
export const OPSLOG_MAX_SIZE_MB_KEY = "opslog.max_size_mb";
export const OPSLOG_BUCKET_POLICIES_KEY = "opslog.bucket_policies";

export const LOG_LEVEL_OPTIONS = ["info", "warn", "error"] as const;

export interface LogRetentionBucketPolicy {
  component: string;
  level: string;
  retention_days: number;
  max_rows: number;
  max_size_mb: number;
}

export const DEFAULT_BUCKET_POLICIES: LogRetentionBucketPolicy[] = [
  { component: "metadata", level: "info", retention_days: 1, max_rows: 100000, max_size_mb: 128 },
  { component: "scanner", level: "info", retention_days: 2, max_rows: 150000, max_size_mb: 192 },
  { component: "metadata", level: "warn", retention_days: 7, max_rows: 250000, max_size_mb: 256 },
  { component: "scanner", level: "warn", retention_days: 7, max_rows: 250000, max_size_mb: 256 },
];

function normalizeBucketLimit(value: unknown): number {
  const parsed =
    typeof value === "number"
      ? value
      : typeof value === "string"
        ? Number.parseInt(value, 10)
        : Number.NaN;
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

export function parseBucketPolicies(raw: string): LogRetentionBucketPolicy[] {
  if (!raw.trim()) {
    return [];
  }

  const parsed = JSON.parse(raw);
  if (!Array.isArray(parsed)) {
    return [];
  }

  return parsed
    .map((entry) => {
      if (!entry || typeof entry !== "object") {
        return null;
      }
      const component = stringifyUnknown((entry as { component?: unknown }).component).trim();
      const level = stringifyUnknown((entry as { level?: unknown }).level)
        .trim()
        .toLowerCase();
      if (!component || !LOG_LEVEL_OPTIONS.includes(level as (typeof LOG_LEVEL_OPTIONS)[number])) {
        return null;
      }
      return {
        component,
        level,
        retention_days: normalizeBucketLimit(
          (entry as { retention_days?: unknown }).retention_days,
        ),
        max_rows: normalizeBucketLimit((entry as { max_rows?: unknown }).max_rows),
        max_size_mb: normalizeBucketLimit((entry as { max_size_mb?: unknown }).max_size_mb),
      };
    })
    .filter((entry): entry is LogRetentionBucketPolicy => entry !== null);
}

export function serializeBucketPolicies(policies: LogRetentionBucketPolicy[]): string {
  const normalized = policies
    .map((policy) => ({
      component: policy.component.trim(),
      level: policy.level.trim().toLowerCase(),
      retention_days: normalizeBucketLimit(policy.retention_days),
      max_rows: normalizeBucketLimit(policy.max_rows),
      max_size_mb: normalizeBucketLimit(policy.max_size_mb),
    }))
    .filter(
      (policy) =>
        policy.component &&
        LOG_LEVEL_OPTIONS.includes(policy.level as (typeof LOG_LEVEL_OPTIONS)[number]),
    );

  return JSON.stringify(normalized);
}
