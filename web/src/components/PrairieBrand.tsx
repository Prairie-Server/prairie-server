import { cn } from "@/lib/utils";
import { useBranding } from "@/hooks/useBranding";

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
  const src = customSrc ?? (isMark ? PRAIRIE_MARK_SRC : PRAIRIE_WORDMARK_SRC);

  return (
    <span className={cn("block shrink-0", !isMark && "overflow-hidden", className)}>
      <img
        src={src}
        alt={serverName}
        className={cn("h-full w-full object-contain", isMark && "rounded-lg", imageClassName)}
      />
    </span>
  );
}
