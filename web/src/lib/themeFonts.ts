import { DEFAULT_THEME, type ThemeId } from "@/lib/themes";

/** Non-default curated themes use Outfit (and theme editor may need extras). */
export function themeNeedsDeferredFonts(theme: ThemeId): boolean {
  return theme !== DEFAULT_THEME;
}

let deferredFontsPromise: Promise<unknown> | null = null;

/**
 * Load Outfit / Manrope / Urbanist (self-hosted) once. Safe to call repeatedly.
 * Default prairie-dusk only needs Sora + Fraunces from fonts-default.css.
 */
export function ensureThemeFontsLoaded(): void {
  if (deferredFontsPromise) return;
  deferredFontsPromise = import("@/styles/fonts-deferred.css");
}
