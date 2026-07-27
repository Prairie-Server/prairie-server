import type { ImgHTMLAttributes } from "react";
import { staticRasterFormats } from "@/lib/staticImageUrl";

export type PictureImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "src"> & {
  /** Canonical raster URL (.png / .webp / .avif). Sibling formats are preferred. */
  src: string;
};

/**
 * Native AVIF → WebP → PNG selection for bundled static assets that ship all
 * three siblings (brand marks, collection-template posters, etc). Sibling URLs
 * come from {@link staticRasterFormats}. Falls back to a plain img when the
 * path is not a raster we can derive siblings for (e.g. SVG).
 *
 * For object-store artwork where PNG/AVIF siblings may be absent, use
 * {@link ArtworkImage} (onError cascade) instead.
 */
export function PictureImage({ src, alt, ...rest }: PictureImageProps) {
  const formats = staticRasterFormats(src);

  if (!formats) {
    return <img src={src} alt={alt} {...rest} />;
  }

  return (
    <picture>
      <source srcSet={formats.avif} type="image/avif" />
      <source srcSet={formats.webp} type="image/webp" />
      <img src={formats.png} alt={alt} {...rest} />
    </picture>
  );
}
