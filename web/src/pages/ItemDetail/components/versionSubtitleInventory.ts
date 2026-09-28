import type { DownloadedSubtitle, VersionSubtitleTrack } from "@/api/types";
import { getLanguageName } from "@/player/utils/languageNames";

export interface VersionSubtitleInventoryRow {
  key: string;
  source: "embedded" | "external" | "downloaded";
  index?: number;
  language: string;
  languageLabel: string;
  title?: string;
  codec?: string;
  provider?: string;
  releaseName?: string;
  score?: number;
  downloadedSubtitleId?: number;
  forced?: boolean;
  hearingImpaired?: boolean;
  default?: boolean;
}

export interface VersionSubtitleInventorySections {
  embedded: VersionSubtitleInventoryRow[];
  external: VersionSubtitleInventoryRow[];
  downloaded: VersionSubtitleInventoryRow[];
}

function compareText(a: string | undefined, b: string | undefined): number {
  return (a ?? "").localeCompare(b ?? "", undefined, { sensitivity: "base" });
}

function compareBuiltInRows(
  a: VersionSubtitleInventoryRow,
  b: VersionSubtitleInventoryRow,
): number {
  const languageCompare = compareText(a.languageLabel, b.languageLabel);
  if (languageCompare !== 0) return languageCompare;
  if (a.forced !== b.forced) return a.forced ? -1 : 1;
  if (a.default !== b.default) return a.default ? -1 : 1;
  return compareText(a.title ?? a.codec, b.title ?? b.codec);
}

function compareDownloadedRows(
  a: VersionSubtitleInventoryRow,
  b: VersionSubtitleInventoryRow,
): number {
  const scoreDiff = (b.score ?? 0) - (a.score ?? 0);
  if (scoreDiff !== 0) return scoreDiff;

  const providerCompare = compareText(a.provider, b.provider);
  if (providerCompare !== 0) return providerCompare;

  return compareText(a.releaseName, b.releaseName);
}

function normalizeBuiltInTrack(
  track: VersionSubtitleTrack,
  index: number,
): VersionSubtitleInventoryRow {
  const source = track.external ? "external" : "embedded";
  const language = track.language?.trim() || "unknown";
  const title = track.title?.trim() || track.embedded_title?.trim() || track.file_name?.trim();

  return {
    key: `${source}:${track.index ?? index}:${language}:${track.codec ?? ""}:${title ?? ""}`,
    source,
    index: track.index ?? index,
    language,
    languageLabel: getLanguageName(language),
    title,
    codec: track.codec?.trim(),
    forced: track.forced,
    hearingImpaired: track.hearing_impaired,
    default: track.default,
  };
}

function normalizeDownloadedSubtitle(subtitle: DownloadedSubtitle): VersionSubtitleInventoryRow {
  const language = subtitle.language?.trim() || "unknown";

  return {
    key: `downloaded:${subtitle.id}`,
    source: "downloaded",
    language,
    languageLabel: getLanguageName(language),
    codec: subtitle.format?.trim(),
    provider: subtitle.provider?.trim(),
    releaseName: subtitle.release_name?.trim(),
    score: subtitle.score,
    downloadedSubtitleId: subtitle.id,
    hearingImpaired: subtitle.hearing_impaired,
  };
}

export function buildVersionSubtitleInventory(
  tracks: VersionSubtitleTrack[] | undefined,
  downloaded: DownloadedSubtitle[] | undefined,
): VersionSubtitleInventorySections {
  const sections: VersionSubtitleInventorySections = {
    embedded: [],
    external: [],
    downloaded: [],
  };

  for (const [index, track] of (tracks ?? []).entries()) {
    const row = normalizeBuiltInTrack(track, index);
    if (row.source === "external") {
      sections.external.push(row);
    } else {
      sections.embedded.push(row);
    }
  }

  for (const subtitle of downloaded ?? []) {
    sections.downloaded.push(normalizeDownloadedSubtitle(subtitle));
  }

  sections.embedded.sort(compareBuiltInRows);
  sections.external.sort(compareBuiltInRows);
  sections.downloaded.sort(compareDownloadedRows);

  return sections;
}
