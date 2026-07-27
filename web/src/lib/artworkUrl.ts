/**
 * Artwork URL helpers mirroring internal/artworkkey.
 * Canonical cache keys stay .webp; clients try AVIF → WebP → PNG so older
 * OSes/devices that cannot decode AVIF or WebP still get a sibling.
 *
 * Sibling derivation is shared with bundled static assets via
 * {@link staticRasterFormats} / {@link staticRasterCandidates}. Prefer
 * {@link PictureImage} when all three siblings are guaranteed on disk
 * (brand marks, collection-template posters); prefer {@link ArtworkImage}
 * when siblings may be missing (object-store artwork).
 *
 * Width variants live in the object key (`/original.`, `/w300.`, `/w500.`, …),
 * not query params. Path rewrite matches the Go artworkkey contract and is
 * skipped for SigV4-style signed URLs (rewriting the path would invalidate
 * the signature).
 */

import { staticRasterCandidates, staticRasterFormats } from "@/lib/staticImageUrl";

export const POSTER_WIDTHS = [300, 500] as const;
export const BACKDROP_WIDTHS = [300, 1280, 1920] as const;

function pathnameOf(objectPath: string): string {
  if (!objectPath.includes("://")) return objectPath;
  try {
    return new URL(objectPath).pathname;
  } catch {
    return "";
  }
}

/**
 * Returns the AVIF sibling for a raster path (.webp / .png / .avif) or URL.
 * Query/fragment are preserved. Non-raster inputs return "".
 */
export function webPAVIFSibling(objectPath: string | null | undefined): string {
  return staticRasterFormats(objectPath)?.avif ?? "";
}

/**
 * Returns the PNG sibling for a raster path (.webp / .png / .avif) or URL.
 * Query/fragment are preserved. Non-raster inputs return "".
 */
export function webPPNGSibling(objectPath: string | null | undefined): string {
  return staticRasterFormats(objectPath)?.png ?? "";
}

/** True when rewriting the path would invalidate a cloud object signature. */
export function isSignedArtworkURL(objectPath: string): boolean {
  return /[?&](X-Amz-Signature|X-Goog-Signature|Signature|sig)=/i.test(objectPath);
}

/**
 * Ordered load candidates for a canonical artwork URL: AVIF → WebP → PNG when
 * siblings can be derived; otherwise just the original URL.
 *
 * Signed URLs return only the original — inventing AVIF/PNG siblings would
 * request an unsigned path and fail before the WebP fallback.
 */
export function artworkCandidates(objectPath: string | null | undefined): string[] {
  const trimmed = objectPath?.trim() ?? "";
  if (!trimmed) return [];
  if (isSignedArtworkURL(trimmed)) return [trimmed];
  return staticRasterCandidates(trimmed);
}

function rewritePathWidthVariant(pathname: string, width: number): string {
  // Basename starts with original. or wN. (optional revision), then extension.
  return pathname.replace(/\/(original|w\d+)(?=\.)/, `/w${width}`);
}

/**
 * Rewrites an artwork URL's width variant segment (`original` / `w300` / …)
 * to `w{width}`. Returns "" when the URL cannot safely be rewritten (signed or
 * unrecognized path shape).
 */
export function artworkWidthVariant(objectPath: string | null | undefined, width: number): string {
  const trimmed = objectPath?.trim() ?? "";
  if (!trimmed || !Number.isFinite(width) || width <= 0) return "";
  if (isSignedArtworkURL(trimmed)) return "";

  if (trimmed.includes("://")) {
    try {
      const u = new URL(trimmed);
      const next = rewritePathWidthVariant(u.pathname, width);
      if (next === u.pathname) return "";
      u.pathname = next;
      return u.toString();
    } catch {
      return "";
    }
  }

  const next = rewritePathWidthVariant(trimmed, width);
  return next === trimmed ? "" : next;
}

/**
 * True when the path already ends at the requested width variant.
 */
function isAlreadyWidth(objectPath: string, width: number): boolean {
  const pathname = pathnameOf(objectPath);
  return new RegExp(`/w${width}(?=\\.)`).test(pathname);
}

/**
 * Builds a `srcSet` string of width variants for the given URL. Skips signed
 * URLs and paths that do not contain a recognizable artwork variant segment.
 * Returns "" when fewer than two distinct candidates exist (no benefit).
 */
export function artworkSrcSet(
  objectPath: string | null | undefined,
  widths: readonly number[],
): string {
  const trimmed = objectPath?.trim() ?? "";
  if (!trimmed || widths.length === 0) return "";
  if (isSignedArtworkURL(trimmed)) return "";

  // Must look like an artwork variant path for any rewrite to make sense.
  const pathname = pathnameOf(trimmed);
  if (!/\/(original|w\d+)(?=\.)/.test(pathname)) return "";

  const parts: string[] = [];
  const seen = new Set<string>();
  for (const width of widths) {
    let url = artworkWidthVariant(trimmed, width);
    if (!url && isAlreadyWidth(trimmed, width)) {
      url = trimmed;
    }
    if (!url || seen.has(url)) continue;
    seen.add(url);
    parts.push(`${url} ${width}w`);
  }

  return parts.length > 1 ? parts.join(", ") : "";
}
