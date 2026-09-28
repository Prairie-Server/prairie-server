/**
 * Helpers for the taste-seeding onboarding flow. Whether a profile has been
 * "seeded" is inferred at runtime by looking at favorites count: a profile
 * with at least one favorite is considered seeded (either by completing the
 * picker or by favoriting items via normal use).
 *
 * The dismissed flag is a localStorage hint that the user explicitly chose to
 * skip the prompt. It suppresses both the post-profile-select redirect and
 * the home page banner so we don't nag them.
 */

export function tasteSeedDismissedKey(profileId: string): string {
  return `taste_seed_dismissed_${profileId}`;
}

export function isTasteSeedDismissed(profileId: string | null | undefined): boolean {
  if (!profileId) return false;
  try {
    return localStorage.getItem(tasteSeedDismissedKey(profileId)) === "true";
  } catch {
    // localStorage can throw in private browsing — assume not dismissed.
    return false;
  }
}

export function setTasteSeedDismissed(profileId: string): void {
  try {
    localStorage.setItem(tasteSeedDismissedKey(profileId), "true");
  } catch {
    // Ignore — this is just UX state.
  }
}

export function clearTasteSeedDismissed(profileId: string): void {
  try {
    localStorage.removeItem(tasteSeedDismissedKey(profileId));
  } catch {
    // Ignore.
  }
}

/**
 * Separate flag for the home page banner that re-prompts skipped users.
 * Lets the user fully opt out of the nudge without affecting other state.
 */
export function tasteSeedBannerDismissedKey(profileId: string): string {
  return `taste_seed_banner_dismissed_${profileId}`;
}

export function isTasteSeedBannerDismissed(profileId: string | null | undefined): boolean {
  if (!profileId) return false;
  try {
    return localStorage.getItem(tasteSeedBannerDismissedKey(profileId)) === "true";
  } catch {
    return false;
  }
}

export function setTasteSeedBannerDismissed(profileId: string): void {
  try {
    localStorage.setItem(tasteSeedBannerDismissedKey(profileId), "true");
  } catch {
    // Ignore.
  }
}
