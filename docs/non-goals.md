# Non-goals

Prairie is an early WIP and most of its scope is open. This document records areas that need special caution before implementation so proposals and PRs can account for product, legal, and app-store risk.

## IPTV and arbitrary remote stream shortcuts

Prairie's roadmap may include Live TV, OTA tuners, guide data, and DVR work. Those areas are no longer categorically banned.

The narrower non-goal is support for arbitrary remote stream playback surfaces that would make Prairie or its first-party clients a general conduit for unvetted remote media URLs, including:

- IPTV playlist ingestion from arbitrary M3U/M3U8 sources.
- Provider portals, Xtream-style APIs, or generic stream relays.
- `.strm` files or equivalent shortcuts whose contents are remote media URLs fetched, proxied, redirected to, or transcoded by the server.

### Why

First-party clients may ship through app stores with strict review expectations for media apps. A server/client pair that plays arbitrary remote stream URLs can be treated as a conduit for unlicensed content regardless of operator intent. That risk is different from controlled Live TV/OTA/DVR integrations with explicit device, guide, and recording models.

### Plugin boundary

Plugins should not be used to bypass this boundary. Any feature that exposes arbitrary remote stream URLs through first-party client UI reproduces the same store-risk problem.

### Acceptable direction

Live TV and DVR designs should be scoped around accountable sources, user-owned tuners, explicit guide data, and clear client UX. Keep arbitrary IPTV/remote URL shortcuts out of core and plugins unless project direction changes explicitly.
