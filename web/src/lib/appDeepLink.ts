/**
 * Deep links into the native Prairie apps via the prairie:// custom scheme.
 *
 * Prairie is self-hosted, so the store apps cannot pre-verify every server's
 * domain for App Links / Universal Links; a custom scheme is the only
 * universal way in. `server` carries the full origin so non-443 ports and
 * plain-http LAN servers need no extra convention.
 *
 * Custom-scheme URLs don't linkify in email or SMS and error when the app is
 * missing, so they are never sent anywhere: they only back an explicit
 * in-page button.
 *
 * NOTE: no caller emits the invite link yet. prairie-android registers
 * prairie:// for the device, item, play and downloads hosts only — there is no
 * invite handling in that app, so a button would fire a link it cannot answer.
 * See the comment in pages/InviteClaim.tsx. The contract is kept here, tested,
 * and ready for the day the app claims the host.
 */

export type MobilePlatform = "android" | "ios";

/** Detects a platform with a native Prairie app from the user agent. */
export function detectMobilePlatform(ua: string): MobilePlatform | null {
  // iPadOS 13+ Safari masquerades as macOS; maxTouchPoints tells it apart,
  // but that's a live-DOM concern — callers pass a UA and we keep this pure.
  if (/android/i.test(ua)) return "android";
  if (/iphone|ipad|ipod/i.test(ua)) return "ios";
  return null;
}

/**
 * Builds the prairie:// deep link that opens the native invite claim flow.
 * Returns null for origins the apps can't talk to (non-http(s), userinfo).
 */
export function buildInviteDeepLink(
  pageOrigin: string,
  token: string,
): string | null {
  let origin: URL;
  try {
    origin = new URL(pageOrigin);
  } catch {
    return null;
  }
  if (origin.username || origin.password) return null;
  if (origin.protocol !== "https:" && origin.protocol !== "http:") return null;
  const server = encodeURIComponent(origin.origin);
  return `prairie://invite?server=${server}&token=${encodeURIComponent(token)}`;
}
