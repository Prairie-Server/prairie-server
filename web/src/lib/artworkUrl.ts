/**
 * Artwork URL helpers mirroring internal/artworkkey.
 * Canonical cache keys stay .webp; clients try AVIF → WebP → PNG so older
 * OSes/devices that cannot decode AVIF or WebP still get a sibling.
 *
 * Width variants live in the object key (`/original.`, `/w300.`, `/w500.`, …),
 * not query params. Path rewrite matches the Go artworkkey contract and is
 * skipped for SigV4-style signed URLs (rewriting the path would invalidate
 * the signature).
 */

export const POSTER_WIDTHS = [300, 500] as const;
export const BACKDROP_WIDTHS = [300, 1280, 1920] as const;

function pathExtension(pathname: string): string {
  const base = pathname.split("/").pop() ?? "";
  const dot = base.lastIndexOf(".");
  if (dot < 0) return "";
  return base.slice(dot);
}

function pathnameOf(objectPath: string): string {
  if (!objectPath.includes("://")) return objectPath;
  try {
    return new URL(objectPath).pathname;
  } catch {
    return "";
  }
}

function webPFormatSibling(objectPath: string | null | undefined, ext: ".avif" | ".png"): string {
  const trimmed = objectPath?.trim() ?? "";
  if (!trimmed) return "";

  if (trimmed.includes("://")) {
    try {
      const u = new URL(trimmed);
      const cur = pathExtension(u.pathname);
      if (cur.toLowerCase() !== ".webp") return "";
      u.pathname = `${u.pathname.slice(0, -cur.length)}${ext}`;
      return u.toString();
    } catch {
      return "";
    }
  }

  const cur = pathExtension(trimmed);
  if (cur.toLowerCase() !== ".webp") return "";
  return `${trimmed.slice(0, -cur.length)}${ext}`;
}

/**
 * Returns the AVIF sibling for a canonical WebP object key or http(s) URL.
 * Query/fragment are preserved. Non-WebP inputs return "".
 */
export function webPAVIFSibling(objectPath: string | null | undefined): string {
  return webPFormatSibling(objectPath, ".avif");
}

/**
 * Returns the PNG sibling for a canonical WebP object key or http(s) URL.
 * Query/fragment are preserved. Non-WebP inputs return "".
 */
export function webPPNGSibling(objectPath: string | null | undefined): string {
  return webPFormatSibling(objectPath, ".png");
}

/**
 * Ordered load candidates for a canonical artwork URL: AVIF → WebP → PNG when
 * the input is WebP; otherwise just the original URL.
 *
 * Bundled static PNG brand assets use {@link staticRasterFormats} / PictureImage
 * instead — those siblings are guaranteed to exist, so a native `<picture>` is
 * preferable to speculative onError fallbacks.
 */
export function artworkCandidates(objectPath: string | null | undefined): string[] {
  const trimmed = objectPath?.trim() ?? "";
  if (!trimmed) return [];

  const avif = webPAVIFSibling(trimmed);
  const png = webPPNGSibling(trimmed);
  const out: string[] = [];
  if (avif) out.push(avif);
  out.push(trimmed);
  if (png) out.push(png);
  return out;
}

/** True when rewriting the path would invalidate a cloud object signature. */
export function isSignedArtworkURL(objectPath: string): boolean {
  return /[?&](X-Amz-Signature|X-Goog-Signature|Signature|sig)=/i.test(objectPath);
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
