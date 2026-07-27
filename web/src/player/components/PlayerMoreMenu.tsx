import { useCallback, useEffect, useRef, useState } from "react";
import { Info, MoreHorizontal, PictureInPicture2, Tags } from "lucide-react";

interface PlayerMoreMenuProps {
  markerEditAvailable?: boolean;
  markerEditActive?: boolean;
  onToggleMarkerEdit?: () => void;
  showPlaybackInfo?: boolean;
  onTogglePlaybackInfo: () => void;
  onTogglePiP?: () => void;
}

/**
 * Mobile-only overflow for secondary player utilities (markers / info / PiP)
 * that are otherwise hidden below the sm breakpoint.
 */
export function PlayerMoreMenu({
  markerEditAvailable,
  markerEditActive,
  onToggleMarkerEdit,
  showPlaybackInfo,
  onTogglePlaybackInfo,
  onTogglePiP,
}: PlayerMoreMenuProps) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const pipEnabled =
    typeof document !== "undefined" && Boolean(onTogglePiP) && document.pictureInPictureEnabled;

  const handleBlur = useCallback((e: React.FocusEvent) => {
    if (!menuRef.current?.contains(e.relatedTarget as Node)) {
      setOpen(false);
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [open]);

  const run = (action: () => void) => {
    action();
    setOpen(false);
  };

  return (
    <div ref={menuRef} className="relative sm:hidden" onBlur={handleBlur}>
      <button
        type="button"
        className="player-utility-btn"
        onClick={() => setOpen((v) => !v)}
        aria-label="More"
        aria-expanded={open}
        aria-haspopup="menu"
        data-active={open || markerEditActive || showPlaybackInfo ? "true" : "false"}
      >
        <MoreHorizontal className="h-[18px] w-[18px]" />
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 bottom-full mb-2 min-w-[180px] rounded-lg bg-black/90 py-1.5 shadow-xl backdrop-blur-sm"
        >
          {markerEditAvailable && onToggleMarkerEdit ? (
            <button
              type="button"
              role="menuitem"
              className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm text-white/85 transition-colors hover:bg-white/10"
              onClick={() => run(onToggleMarkerEdit)}
              aria-pressed={markerEditActive}
            >
              <Tags className="h-4 w-4 shrink-0" />
              {markerEditActive ? "Done editing markers" : "Edit markers"}
            </button>
          ) : null}

          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm text-white/85 transition-colors hover:bg-white/10"
            onClick={() => run(onTogglePlaybackInfo)}
            aria-pressed={showPlaybackInfo}
          >
            <Info className="h-4 w-4 shrink-0" />
            Playback info
          </button>

          {pipEnabled && onTogglePiP ? (
            <button
              type="button"
              role="menuitem"
              className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm text-white/85 transition-colors hover:bg-white/10"
              onClick={() => run(onTogglePiP)}
            >
              <PictureInPicture2 className="h-4 w-4 shrink-0" />
              Picture in Picture
            </button>
          ) : null}
        </div>
      )}
    </div>
  );
}
