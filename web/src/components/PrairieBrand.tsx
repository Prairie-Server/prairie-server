import { cn } from "@/lib/utils";
import { useBranding } from "@/hooks/useBranding";
import { PictureImage } from "@/components/PictureImage";

const PRAIRIE_WORDMARK_SRC = "/prairie-wordmark-sidebar.png";
const PRAIRIE_MARK_SRC = "/prairie-icon-1024.png";

export type PrairieBrandVariant = "wordmark" | "mark";

interface PrairieBrandProps {
  className?: string;
  imageClassName?: string;
  variant?: PrairieBrandVariant;
}

/**
 * Brand mark / wordmark. The bundled mark is the same 1024² app icon used for
 * favicon / PWA icons (sharp on retina at ~44px). The bundled wordmark is that
 * mark plus a "Prairie" lockup on a transparent strip (~274×90). Prefer a
 * custom upload via branding settings for alternate wordmarks. Bundled assets
 * use PictureImage (AVIF → WebP → PNG); custom assets are content-negotiated by
 * the branding API.
 */
export function PrairieBrand({
  className,
  imageClassName,
  variant = "wordmark",
}: PrairieBrandProps) {
  const isMark = variant === "mark";
  const { serverName, wordmarkUrl, markUrl } = useBranding();

  const customSrc = isMark ? markUrl : wordmarkUrl;
  const defaultSrc = isMark ? PRAIRIE_MARK_SRC : PRAIRIE_WORDMARK_SRC;
  const imageClass = cn(
    "h-full w-full object-contain",
    isMark && "rounded-lg",
    imageClassName,
  );
  // Intrinsic pixel size of the bundled assets — helps decode prioritization
  // on high-DPI displays when CSS sizes the element down.
  const intrinsic = isMark
    ? { width: 1024, height: 1024 }
    : { width: 274, height: 90 };

  return (
    <span
      className={cn("block shrink-0", !isMark && "overflow-hidden", className)}
    >
      {customSrc ? (
        <img
          src={customSrc}
          alt={serverName}
          decoding="async"
          className={imageClass}
        />
      ) : (
        <PictureImage
          src={defaultSrc}
          alt={serverName}
          decoding="async"
          width={intrinsic.width}
          height={intrinsic.height}
          className={imageClass}
        />
      )}
    </span>
  );
}
