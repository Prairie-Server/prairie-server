import { getAccessToken, getProfileId, getProfileToken } from "@/api/client";

/**
 * Releases a live session during page teardown, where React cleanup and normal
 * promises never run. `keepalive` lets the request outlive the document; without
 * it the tuner stays claimed until the server's stale-session sweep.
 */
export function releaseLiveTVSessionOnUnload(sessionId: string): void {
  if (!sessionId || typeof fetch !== "function") return;
  const headers: Record<string, string> = {};
  const token = getAccessToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const profileId = getProfileId();
  if (profileId) headers["X-Profile-Id"] = profileId;
  const profileToken = getProfileToken();
  if (profileToken) headers["X-Profile-Token"] = profileToken;
  void fetch(`/api/v1/livetv/sessions/${encodeURIComponent(sessionId)}`, {
    method: "DELETE",
    headers,
    keepalive: true,
  }).catch(() => undefined);
}

/** Fullscreen Live TV watch route — same shell as movies/shows (`/watch/...`). */
export function buildLiveWatchHref(
  channelId: string,
  returnHref = "/livetv",
): string {
  const id = encodeURIComponent(channelId);
  const params = new URLSearchParams();
  if (returnHref) {
    params.set("return", returnHref);
  }
  const query = params.toString();
  return `/watch/live/${id}${query ? `?${query}` : ""}`;
}
