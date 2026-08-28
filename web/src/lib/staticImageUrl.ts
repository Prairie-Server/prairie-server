/**
 * Helpers for bundled static images that ship AVIF + WebP + PNG siblings.
 * Prefer AVIF, then WebP, then PNG (widest decode support).
 */

import { getImageFormats, orderRasterCandidates } from "@/lib/imageFormats";

function pathExtension(pathname: string): string {
  const base = pathname.split("/").pop() ?? "";
  const dot = base.lastIndexOf(".");
  if (dot < 0) return "";
  return base.slice(dot);
}

function splitPathAndQuery(src: string): { path: string; query: string } {
  if (src.includes("://")) {
    try {
      const u = new URL(src);
      return { path: u.pathname, query: `${u.search}${u.hash}` };
    } catch {
      return { path: src, query: "" };
    }
  }
  const q = src.search(/[?#]/);
  if (q < 0) return { path: src, query: "" };
  return { path: src.slice(0, q), query: src.slice(q) };
}

export type StaticRasterFormats = {
  avif: string;
  webp: string;
  png: string;
};

/**
 * Derive AVIF / WebP / PNG URLs from a raster path that uses any of those
 * extensions as the canonical file. Returns null for non-raster paths (svg, ico).
 */
export function staticRasterFormats(src: string | null | undefined): StaticRasterFormats | null {
  const trimmed = src?.trim() ?? "";
  if (!trimmed) return null;

  if (trimmed.includes("://")) {
    try {
      const u = new URL(trimmed);
      const ext = pathExtension(u.pathname).toLowerCase();
      if (ext !== ".png" && ext !== ".webp" && ext !== ".avif") return null;
      const stem = u.pathname.slice(0, -ext.length);
      const withExt = (next: ".avif" | ".webp" | ".png") => {
        const copy = new URL(u.toString());
        copy.pathname = `${stem}${next}`;
        return copy.toString();
      };
      return { avif: withExt(".avif"), webp: withExt(".webp"), png: withExt(".png") };
    } catch {
      return null;
    }
  }

  const { path, query } = splitPathAndQuery(trimmed);
  const ext = pathExtension(path).toLowerCase();
  if (ext !== ".png" && ext !== ".webp" && ext !== ".avif") return null;
  const stem = path.slice(0, -ext.length);
  return {
    avif: `${stem}.avif${query}`,
    webp: `${stem}.webp${query}`,
    png: `${stem}.png${query}`,
  };
}

/** Ordered load candidates using the client's detected raster preference. */
export function staticRasterCandidates(src: string | null | undefined): string[] {
  const formats = staticRasterFormats(src);
  if (!formats) {
    const trimmed = src?.trim() ?? "";
    return trimmed ? [trimmed] : [];
  }
  return orderRasterCandidates(formats, getImageFormats());
}
