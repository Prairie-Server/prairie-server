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
      {customSrc ? (
        // Custom branding assets are content-negotiated by the server (Accept).
        <img src={customSrc} alt={serverName} className={imageClass} />
      ) : (
        <PictureImage src={defaultSrc} alt={serverName} className={imageClass} />
      )}
    </span>
  );
}
