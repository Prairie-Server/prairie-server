import { useState, type ImgHTMLAttributes } from "react";
import { artworkCandidates, artworkSrcSet, type ArtworkFormatSources } from "@/lib/artworkUrl";

export type ArtworkImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "srcSet"> & {
  /** Canonical artwork URL (typically a .webp object key or signed URL). */
  src: string | null | undefined;
  /** Pre-signed AVIF sibling from the API (preferred for signed CDN URLs). */
  avifSrc?: string | null;
  /** Pre-signed PNG sibling from the API. */
  pngSrc?: string | null;
  /**
   * Width ladder for `srcSet` (e.g. POSTER_WIDTHS / BACKDROP_WIDTHS).
   * When omitted, renders a single `src` with format fallback only.
   */
  widths?: readonly number[];
};

/**
 * Prefers the AVIF sibling of a canonical WebP artwork URL, then WebP, then
 * PNG when earlier formats are missing or fail to load (legacy caches, older
 * OSes/devices without AVIF/WebP decode).
 *
 * Prefer passing `avifSrc` / `pngSrc` from the API for signed object URLs —
 * path rewriting cannot mint valid SigV4 / Cloudflare token siblings.
 *
 * Optional `widths` + `sizes` emit a responsive `srcSet` by rewriting the
 * object-key width variant (`original` / `w300` / …). Signed URLs skip srcset
 * so the signature stays valid.
 */
export function ArtworkImage({
  src,
  avifSrc,
  pngSrc,
  alt,
  widths,
  sizes,
  onError,
  onLoad,
  ...rest
}: ArtworkImageProps) {
  const formats: Omit<ArtworkFormatSources, "src"> = { avif: avifSrc, png: pngSrc };
  const candidates = artworkCandidates(src, formats);
  const [failedCount, setFailedCount] = useState(0);

  // Reset fallback state when the canonical URL changes (render-time adjust).
  const [prevSrc, setPrevSrc] = useState(src);
  if (src !== prevSrc) {
    setPrevSrc(src);
    setFailedCount(0);
  }

  if (!src || candidates.length === 0) return null;

  const index = Math.min(failedCount, candidates.length - 1);
  const current = candidates[index]!;
  const srcSet = widths && widths.length > 0 ? artworkSrcSet(current, widths) : "";

  return (
    <img
      {...rest}
      src={current}
      srcSet={srcSet || undefined}
      sizes={srcSet ? sizes : undefined}
      alt={alt}
      onLoad={onLoad}
      onError={(event) => {
        if (failedCount < candidates.length - 1) {
          setFailedCount((n) => n + 1);
          return;
        }
        onError?.(event);
      }}
    />
  );
}
