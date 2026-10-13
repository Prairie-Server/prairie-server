import { v2 } from "@/api/v2/request";

/**
 * Releases a live session during page teardown, where React cleanup and normal
 * promises never run. `keepalive` lets the request outlive the document; without
 * it the tuner stays claimed until the server's stale-session sweep.
 */
export function releaseLiveTVSessionOnUnload(sessionId: string): void {
  if (!sessionId || typeof fetch !== "function") return;
  void v2("DELETE /api/v2/livetv/sessions/{session_id}", {
    path: { session_id: sessionId },
    keepalive: true,
    retryAuthentication: false,
  }).catch(() => undefined);
}

/** Fullscreen Live TV watch route — same shell as movies/shows (`/watch/...`). */
export function buildLiveWatchHref(channelId: string, returnHref = "/livetv"): string {
  const id = encodeURIComponent(channelId);
  const params = new URLSearchParams();
  if (returnHref) {
    params.set("return", returnHref);
  }
  const query = params.toString();
  return `/watch/live/${id}${query ? `?${query}` : ""}`;
}
