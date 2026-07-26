/**
 * Artwork URL helpers mirroring internal/artworkkey.
 * Canonical cache keys stay .webp; clients try AVIF → WebP → PNG so older
 * OSes/devices that cannot decode AVIF or WebP still get a sibling.
 */

function pathExtension(pathname: string): string {
  const base = pathname.split("/").pop() ?? "";
  const dot = base.lastIndexOf(".");
  if (dot < 0) return "";
  return base.slice(dot);
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
