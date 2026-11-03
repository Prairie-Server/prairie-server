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

## Deleting or managing media files

**Out of scope permanently.** Prairie is a media server, not a media manager. It reads the files in
a library; it does not delete, move, rename, or reorganize them. This covers, at minimum:

- Deleting a media file or item from disk, through any API, admin option, or client action.
- Moving, renaming, or sorting files into a folder layout.
- Automatic cleanup, such as removing an episode once it has been watched.

### Why

**The library belongs to the operator.** Prairie only needs read access to media; the shipped Compose file mounts libraries read-only. A server that can
delete files needs write access to every library, and then one bug, compromised account, or
mistaken tap can destroy media that may not be backed up. Keeping Prairie read-only removes that
whole class of failure.

**Other tools already do this well.** Library managers such as Sonarr and Radarr, and the
operator's own file manager or shell, own file layout and cleanup. When they change files, Prairie's
scanner picks up the result.

### Not available as a plugin either

A plugin that deletes or moves library files brings back the same write access and the same risk.
Not in core, not in a plugin, not in a client. The Jellyfin compatibility layer reports items as
not deletable for the same reason.

### What to do instead

Manage files with the tools built for it: a library manager, a file manager, or the shell. Prairie
reflects the change on its next scan.
