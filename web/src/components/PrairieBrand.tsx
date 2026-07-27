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
 * Brand mark / wordmark. The bundled mark is 1024² (sharp on retina at ~44px);
 * the bundled wordmark is a modest 142×96 PNG — prefer a custom upload via
 * branding settings for crisp wordmarks on high-DPI displays.
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
  const imageClass = cn("h-full w-full object-contain", isMark && "rounded-lg", imageClassName);

  return (
    <span className={cn("block shrink-0", !isMark && "overflow-hidden", className)}>
<<<<<<< HEAD
      {customSrc ? (
        // Custom branding assets are content-negotiated by the server (Accept).
        <img src={customSrc} alt={serverName} className={imageClass} />
      ) : (
        <PictureImage src={defaultSrc} alt={serverName} className={imageClass} />
      )}
=======
      <img
        src={src}
        alt={serverName}
        decoding="async"
        // Bundled mark is large enough for 3×; hint display size so the browser
        // can prioritize decode. Custom branding URLs keep single-src behavior.
        width={isMark ? 1024 : 142}
        height={isMark ? 1024 : 96}
        className={cn("h-full w-full object-contain", isMark && "rounded-lg", imageClassName)}
      />
>>>>>>> 0c2eafb9 (feat(web): finish mobile responsive deferred follow-ups)
    </span>
  );
}
