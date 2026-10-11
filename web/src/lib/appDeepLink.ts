/**
 * Deep links into the native Prairie apps via the prairie:// custom scheme.
 *
 * Prairie is self-hosted, so the store apps cannot pre-verify every server's
 * domain for App Links / Universal Links; a custom scheme is the only
 * universal way in. `server` carries the full origin so non-443 ports and
 * plain-http LAN servers need no extra convention. Watch Party joins use
 * the same shape under the `watch-party` host.
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
export function buildInviteDeepLink(pageOrigin: string, token: string): string | null {
  return buildServerDeepLink("invite", pageOrigin, token);
}

/**
 * Builds the prairie:// deep link that joins a Watch Party by its invite token:
 * `prairie://watch-party?server=<url>&token=<token>`. The Apple app registers it
 * (prairie-apple); Android does not yet. Same origin rules as invites.
 */
export function buildWatchPartyDeepLink(pageOrigin: string, token: string): string | null {
  return buildServerDeepLink("watch-party", pageOrigin, token);
}

/**
 * Builds the prairie:// deep link that opens a TV sign-in approval in the app:
 * `prairie://device?server=<server_id>&url=<origin>&code=<code>`. `server` is
 * the deployment identity (getServerIdentity), which the apps match against
 * their saved servers because one server has several addresses; it is left
 * out when unknown, and the apps then match `url`. The code is sent without
 * its separator. Both the Apple and Android apps register it; the format is
 * in docs/architecture/device-login.md.
 */
export function buildDeviceDeepLink(
  serverId: string | undefined,
  pageOrigin: string,
  code: string,
): string | null {
  const origin = httpOrigin(pageOrigin);
  const compact = code.replace(/[^A-Za-z0-9]/g, "");
  if (!origin || !compact) return null;
  const query = [
    serverId ? `server=${encodeURIComponent(serverId)}` : "",
    `url=${encodeURIComponent(origin)}`,
    `code=${encodeURIComponent(compact)}`,
  ].filter(Boolean);
  return `prairie://device?${query.join("&")}`;
}

/** The origin of an http(s) page the apps can talk to, or null. */
function httpOrigin(pageOrigin: string): string | null {
  let origin: URL;
  try {
    origin = new URL(pageOrigin);
  } catch {
    return null;
  }
  if (origin.username || origin.password) return null;
  if (origin.protocol !== "https:" && origin.protocol !== "http:") return null;
  return origin.origin;
}

function buildServerDeepLink(host: string, pageOrigin: string, token: string): string | null {
  const origin = httpOrigin(pageOrigin);
  if (!origin || !token) return null;
  const server = encodeURIComponent(origin);
  return `prairie://${host}?server=${server}&token=${encodeURIComponent(token)}`;
}
