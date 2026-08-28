import { describe, expect, it } from "vitest";
import { ensureThemeFontsLoaded, themeNeedsDeferredFonts } from "./themeFonts";

describe("themeFonts", () => {
  it("only loads deferred fonts for non-default themes", () => {
    expect(themeNeedsDeferredFonts("prairie-dusk")).toBe(false);
    expect(themeNeedsDeferredFonts("midnight-cinema")).toBe(true);
    expect(themeNeedsDeferredFonts("cobalt-studio")).toBe(true);
  });

  it("ensureThemeFontsLoaded is safe to call repeatedly", () => {
    expect(() => {
      ensureThemeFontsLoaded();
      ensureThemeFontsLoaded();
    }).not.toThrow();
  });
});
