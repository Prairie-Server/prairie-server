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
import { getImageFormats, orderRasterCandidates } from "@/lib/imageFormats";

/**
 * Poster rungs offered to the browser, narrowest first.
 *
 * w200 covers a 140-160px card at DPR 1, w300 the same card on a 2x display at
 * the smaller end, w500 the detail-page poster. The browser picks using the
 * `sizes` attribute each call site declares.
 */
export const POSTER_WIDTHS = [200, 300, 500] as const;
export const BACKDROP_WIDTHS = [300, 1280, 1920] as const;

/**
 * Portrait rungs. Cast cards render 160px and the person page 140-180px, so a
 * 1x display is served w200 and a 2x one w300/w500 — where every one of these
 * used to be w500, roughly 45 KB per face on a page that shows a dozen.
 */
export const PROFILE_WIDTHS = [200, 300, 500] as const;

/**
 * Episode still rungs. Stills have no w200: 140px at 2x needs 280, and the
 * television surface renders ~358px, so w300 is the narrowest rung that covers
 * a real still without upscaling. Mirrors artworkkey.VariantWidths("still").
 */
export const STILL_WIDTHS = [300, 500] as const;

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

/**
 * True when rewriting the path would invalidate a signature we cannot reproduce.
 *
 * Third-party signatures (S3 SigV4, GCS, Cloudflare) cover the exact object
 * path, so any rewrite breaks them and the URL must be used verbatim.
 *
 * Prairie's own artwork signature is deliberately excluded. It covers the
 * artwork *revision*, not the exact key, so selecting a different width rung of
 * the same image still validates — which is what makes `artworkSrcSet` and the
 * width ladders work at all. Before that, this guard matched Prairie's `sig=`
 * too, so every responsive path in this app was dead: no `srcSet`, no `sizes`,
 * and 140-160px cards rendering w500.
 */
export function isSignedArtworkURL(objectPath: string): boolean {
  if (isPrairieSignedArtworkURL(objectPath)) return false;
  // AWS SigV4, GCS, generic Signature, and Cloudflare WAF token (?verify=).
  return /[?&](X-Amz-Signature|X-Goog-Signature|Signature|sig|verify)=/i.test(objectPath);
}

/**
 * True for a URL signed by this server's artwork store.
 *
 * Identified by the pair of query params it always emits together plus the
 * `/artwork/` path prefix it always serves from — deliberately narrow, so a
 * third-party URL that happens to carry a `sig=` param is not mistaken for ours
 * and rewritten into a 403.
 */
export function isPrairieSignedArtworkURL(objectPath: string): boolean {
  if (!/[?&]sig=/.test(objectPath) || !/[?&]expires=/.test(objectPath)) return false;
  return pathnameOf(objectPath).includes("/artwork/");
}

export type ArtworkFormatSources = {
  /** Canonical artwork URL (typically .webp). */
  src?: string | null;
  /** Pre-signed AVIF sibling from the API (preferred when present). */
  avif?: string | null;
  /** Pre-signed PNG sibling from the API. */
  png?: string | null;
};

/**
 * Ordered load candidates for a canonical artwork URL: AVIF → WebP → PNG when
 * siblings can be derived; otherwise just the original URL.
 *
 * When the API supplies already-signed `avif` / `png` URLs (required for
 * SigV4 / Cloudflare token auth), those take precedence over path rewriting.
 *
 * Signed URLs without explicit format siblings return only the original —
 * inventing AVIF/PNG siblings would request an unsigned path and fail before
 * the WebP fallback.
 */
export function artworkCandidates(
  objectPath: string | null | undefined,
  formats?: Omit<ArtworkFormatSources, "src">,
): string[] {
  const trimmed = objectPath?.trim() ?? "";
  const avif = formats?.avif?.trim() ?? "";
  const png = formats?.png?.trim() ?? "";
  if (avif || png) {
    return orderRasterCandidates({ avif, webp: trimmed, png }, getImageFormats());
  }
  if (!trimmed) return [];
  if (isSignedArtworkURL(trimmed)) return [trimmed];
  return staticRasterCandidates(trimmed);
}

/**
 * Best immediate artwork URL for this client without trial-and-error probing.
 * Falls back to the canonical URL when no sibling matches the detected formats.
 */
export function artworkPreferred(
  objectPath: string | null | undefined,
  formats?: Omit<ArtworkFormatSources, "src">,
): string {
  return artworkCandidates(objectPath, formats)[0] ?? "";
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
