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
