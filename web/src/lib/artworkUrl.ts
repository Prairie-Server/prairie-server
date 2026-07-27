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
 */

import { staticRasterCandidates, staticRasterFormats } from "@/lib/staticImageUrl";

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
 * Ordered load candidates for a canonical artwork URL: AVIF → WebP → PNG when
 * siblings can be derived; otherwise just the original URL.
 */
export function artworkCandidates(objectPath: string | null | undefined): string[] {
  return staticRasterCandidates(objectPath);
}
