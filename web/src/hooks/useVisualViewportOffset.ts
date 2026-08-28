import { useEffect, useState } from "react";

export interface VisualViewportOffset {
  /** Pixels the fixed bottom chrome should lift to clear the soft keyboard. */
  bottomOffset: number;
  /** Visible viewport height (for consumers that need it). */
  height: number;
}

/**
 * Tracks `visualViewport` so fixed bottom chrome can clear the soft keyboard
 * on mobile browsers. Returns 0 offset when the keyboard is hidden or the API
 * is unavailable.
 */
export function useVisualViewportOffset(): VisualViewportOffset {
  const [offset, setOffset] = useState<VisualViewportOffset>({
    bottomOffset: 0,
    height: typeof window !== "undefined" ? window.innerHeight : 0,
  });

  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;

    const update = () => {
      // Keyboard (or browser chrome) eats space between layout viewport bottom
      // and visual viewport bottom. Lift fixed chrome by that delta.
      const keyboardInset = Math.max(
        0,
        window.innerHeight - vv.height - vv.offsetTop,
      );
      setOffset({
        bottomOffset: Math.round(keyboardInset),
        height: vv.height,
      });
    };

    update();
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    return () => {
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
    };
  }, []);

  return offset;
}
