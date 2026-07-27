import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useVisualViewportOffset } from "./useVisualViewportOffset";

describe("useVisualViewportOffset", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reports keyboard inset from visualViewport", () => {
    const listeners = new Map<string, Set<() => void>>();
    const vv = {
      height: 400,
      offsetTop: 0,
      addEventListener: (type: string, cb: () => void) => {
        const set = listeners.get(type) ?? new Set();
        set.add(cb);
        listeners.set(type, set);
      },
      removeEventListener: (type: string, cb: () => void) => {
        listeners.get(type)?.delete(cb);
      },
    };
    vi.stubGlobal("visualViewport", vv);
    vi.stubGlobal("innerHeight", 800);

    const { result } = renderHook(() => useVisualViewportOffset());
    expect(result.current.bottomOffset).toBe(400);

    act(() => {
      vv.height = 800;
      listeners.get("resize")?.forEach((cb) => cb());
    });
    expect(result.current.bottomOffset).toBe(0);
  });
});
