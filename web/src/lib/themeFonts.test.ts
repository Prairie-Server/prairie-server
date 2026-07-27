import { describe, expect, it, beforeEach, afterEach } from "vitest";
import { ensureThemeFontsLoaded, themeNeedsDeferredFonts } from "./themeFonts";

describe("themeFonts", () => {
  it("only loads deferred fonts for non-default themes", () => {
    expect(themeNeedsDeferredFonts("prairie-dusk")).toBe(false);
    expect(themeNeedsDeferredFonts("midnight-cinema")).toBe(true);
    expect(themeNeedsDeferredFonts("cobalt-studio")).toBe(true);
  });

  describe("ensureThemeFontsLoaded", () => {
    beforeEach(() => {
      document.body.innerHTML = "";
    });
    afterEach(() => {
      document.body.innerHTML = "";
    });

    it("activates the deferred stylesheet once", () => {
      const link = document.createElement("link");
      link.id = "prairie-theme-fonts";
      link.rel = "stylesheet";
      link.media = "print";
      document.head.appendChild(link);

      ensureThemeFontsLoaded();
      expect(link.media).toBe("all");
      ensureThemeFontsLoaded();
      expect(link.media).toBe("all");
    });

    it("no-ops when the link is absent", () => {
      expect(() => ensureThemeFontsLoaded()).not.toThrow();
    });
  });
});
