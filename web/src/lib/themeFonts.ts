import { DEFAULT_THEME, type ThemeId } from "@/lib/themes";

const THEME_FONTS_LINK_ID = "prairie-theme-fonts";

/** Non-default curated themes use Outfit (and theme editor may need extras). */
export function themeNeedsDeferredFonts(theme: ThemeId): boolean {
  return theme !== DEFAULT_THEME;
}

/**
 * Flip the deferred Google Fonts stylesheet from media=print to all.
 * Safe to call repeatedly; no-ops when the link is missing or already active.
 */
export function ensureThemeFontsLoaded(): void {
  const link = document.getElementById(THEME_FONTS_LINK_ID);
  if (!(link instanceof HTMLLinkElement)) return;
  if (link.media === "all") return;
  link.media = "all";
}
