import { useState, type ImgHTMLAttributes } from "react";
import { artworkCandidates } from "@/lib/artworkUrl";

export type ArtworkImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "src"> & {
  /** Canonical artwork URL (typically a .webp object key or signed URL). */
  src: string | null | undefined;
};

/**
 * Prefers the AVIF sibling of a canonical WebP artwork URL, then WebP, then
 * PNG when earlier formats are missing or fail to load (legacy caches, older
 * OSes/devices without AVIF/WebP decode).
 */
export function ArtworkImage({ src, alt, onError, onLoad, ...rest }: ArtworkImageProps) {
  const candidates = artworkCandidates(src);
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

  return (
    <img
      {...rest}
      src={current}
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
