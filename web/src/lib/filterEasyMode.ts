import type { FilterConfig, FilterGroup, FilterRule } from "@/api/types";

export interface FilterChipModel {
  field: string;
  op: string;
  value: FilterRule["value"];
}

export type ConversionResult =
  | { kind: "compatible"; chips: FilterChipModel[]; matchMode: "all" | "any" }
  | { kind: "incompatible"; reason: string };

/**
 * Wrap a flat chip list in a FilterConfig with a single group. The top-level
 * match is always "all" since there's only one group; the group-level match
 * controls how chips combine (AND vs OR).
 */
export function chipsToFilterConfig(
  chips: FilterChipModel[],
  matchMode: "all" | "any",
): FilterConfig {
  const rules: FilterRule[] = chips.map((c) => ({
    field: c.field,
    op: c.op,
    value: c.value,
  }));
  const group: FilterGroup = { match: matchMode, rules };
  return { match: "all", groups: [group] };
}

/**
 * Convert a FilterConfig back into a flat chip list. Easy Mode supports only
 * single-group configs; nested or multi-group configs are reported as
 * incompatible so the UI can fall back to Advanced Mode.
 */
export function filterConfigToChips(
  config: FilterConfig | undefined | null,
): ConversionResult {
  if (!config || !Array.isArray(config.groups) || config.groups.length === 0) {
    return { kind: "compatible", chips: [], matchMode: "all" };
  }
  if (config.groups.length > 1) {
    return { kind: "incompatible", reason: "multiple groups" };
  }
  const group = config.groups[0];
  if (!group) {
    return { kind: "compatible", chips: [], matchMode: "all" };
  }
  if (group.match !== "all" && group.match !== "any") {
    return { kind: "incompatible", reason: "unknown group match mode" };
  }
  const chips = group.rules.map((r) => ({
    field: r.field,
    op: r.op,
    value: r.value,
  }));
  return { kind: "compatible", chips, matchMode: group.match };
}
