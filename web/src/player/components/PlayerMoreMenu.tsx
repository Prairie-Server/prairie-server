import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
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
 *
 * The popup is portaled to document.body so it is not clipped by the utility
 * rail's overflow-x-auto scroll container.
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
  const [menuPos, setMenuPos] = useState<{ bottom: number; right: number } | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const pipEnabled =
    typeof document !== "undefined" && Boolean(onTogglePiP) && document.pictureInPictureEnabled;

  const updateMenuPos = useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    setMenuPos({
      bottom: window.innerHeight - rect.top + 8,
      right: window.innerWidth - rect.right,
    });
  }, []);

  useLayoutEffect(() => {
    if (!open) {
      setMenuPos(null);
      return;
    }
    updateMenuPos();
    window.addEventListener("resize", updateMenuPos);
    // Capture scroll from overflow ancestors (utility rail) as well as window.
    window.addEventListener("scroll", updateMenuPos, true);
    return () => {
      window.removeEventListener("resize", updateMenuPos);
      window.removeEventListener("scroll", updateMenuPos, true);
    };
  }, [open, updateMenuPos]);

  useEffect(() => {
    if (!open) return;

    const handlePointerDown = (e: PointerEvent) => {
      const target = e.target as Node;
      if (triggerRef.current?.contains(target) || menuRef.current?.contains(target)) {
        return;
      }
      setOpen(false);
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  const run = (action: () => void) => {
    action();
    setOpen(false);
  };

  return (
    <div className="relative sm:hidden">
      <button
        ref={triggerRef}
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

      {open &&
        menuPos &&
        createPortal(
          <div
            ref={menuRef}
            role="menu"
            className="fixed z-[80] min-w-[180px] rounded-lg bg-black/90 py-1.5 shadow-xl backdrop-blur-sm"
            style={{ bottom: menuPos.bottom, right: menuPos.right }}
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
          </div>,
          document.body,
        )}
    </div>
  );
}
