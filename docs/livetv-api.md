# Live TV API (v2)

Live TV is a Prairie-only domain. The `/api/v2/livetv` operations are the native
contract for viewers; tuner, channel and guide-source administration lives
under `/api/v2/admin/livetv`. Both run the same service code as the frozen
`/api/v1/livetv` routes, which keep working unchanged.

Every operation is registered whether or not Live TV is configured. On a
server without Live TV each returns `409 capability_not_configured`.

## Viewer operations

Viewer mutations need an account token plus `X-Profile-Id`. Reads may omit
the profile unless the household has a restricted profile (then `422`). Mutations are
refused in demo mode. Recordings, series rules and sessions belong to the
caller's profile; administrators may see and act on all of them.

| Operation | Method and path | Success | Replaces |
| --- | --- | --- | --- |
| `listLiveTVChannels` | GET `/api/v2/livetv/channels` | 200 `{items: [channel]}` | GET `/api/v1/livetv/channels` |
| `getLiveTVGuide` | GET `/api/v2/livetv/guide` | 200 `{items: [program], start, end}` | GET `/api/v1/livetv/guide` |
| `getLiveTVProgram` | GET `/api/v2/livetv/programs/{program_id}` | 200 program | GET `/api/v1/livetv/programs/{programId}` |
| `startLiveTVSession` | POST `/api/v2/livetv/channels/{channel_id}/sessions` | 201 session | POST `/api/v1/livetv/channels/{channelId}/session` |
| `heartbeatLiveTVSession` | POST `/api/v2/livetv/sessions/{session_id}/heartbeat` | 204 | POST `/api/v1/livetv/sessions/{sessionId}/heartbeat` |
| `stopLiveTVSession` | DELETE `/api/v2/livetv/sessions/{session_id}` | 204 | DELETE `/api/v1/livetv/sessions/{sessionId}` |
| `listLiveTVRecordings` | GET `/api/v2/livetv/recordings` | 200 `{items: [recording]}` | GET `/api/v1/livetv/recordings` |
| `scheduleLiveTVRecording` | POST `/api/v2/livetv/recordings` | 201 recording | POST `/api/v1/livetv/recordings` |
| `cancelLiveTVRecording` | DELETE `/api/v2/livetv/recordings/{recording_id}` | 200 recording | DELETE `/api/v1/livetv/recordings/{recordingId}` |
| `listLiveTVSeriesRules` | GET `/api/v2/livetv/series-rules` | 200 `{items: [rule]}` | GET `/api/v1/livetv/series-rules` |
| `createLiveTVSeriesRule` | POST `/api/v2/livetv/series-rules` | 201 rule | POST `/api/v1/livetv/series-rules` |
| `deleteLiveTVSeriesRule` | DELETE `/api/v2/livetv/series-rules/{rule_id}` | 204 | DELETE `/api/v1/livetv/series-rules/{ruleId}` |

Channels never carry an upstream tuner URL; a client plays a channel by
starting a session. The guide takes repeated `channel_id` query parameters
(at most 500) and optional `start` and `end` (RFC 3339). It defaults to now
through 12 hours later; `end` must be after `start` and the window at most 7
days. Recordings filter on `status`: `scheduled`, `recording`, `completed`,
`failed` or `cancelled`.

### Sessions and the stream URL

`startLiveTVSession` takes an optional body `{client_capabilities:
{codecs_video, codecs_audio, audio_passthrough_codecs, max_audio_channels,
max_resolution}}` and claims a tuner. The response is:

| Field | Meaning |
| --- | --- |
| `session_id` | Pass to heartbeat and stop |
| `channel_id` | The channel played |
| `playback_ticket` | Playback session the stream is delivered under; equals `session_id` without a bridge |
| `stream_url` | Opaque, server-relative play URL; hand it to the player as is |
| `transport` | `hls` (live playlist) or `mpegts` (raw transport-stream proxy) |
| `note` | Optional human-readable planner note |
| `status`, `created_at` | Session state and start time |
| `stale_after_seconds` | Heartbeat well within this (90) while the player is open, including paused |

Live media bytes are served from the same URLs as v1, so `stream_url` points
under `/api/v1/livetv`:

- `hls`: `/api/v1/livetv/live-hls/{id}/index.m3u8?st=...` carries a stream token
  bound to the delivery, so a native player can fetch the playlist and segments
  without headers.
- `mpegts`: `/api/v1/livetv/sessions/{session_id}/stream` needs the bearer token
  and `X-Profile-Id` headers.

Treat `stream_url` as opaque: the server may move live delivery under `/api/v2`
later without a contract change. Stop the session when playback ends; a
session neither fetched nor heartbeated for `stale_after_seconds` is reclaimed.

A start with no free tuner is `409 conflict`; an unknown channel is `404`.

## Administrator operations

Administrator operations require the server-wide `admin` role (see the
[admin API](admin-api.md)); mutations are refused in demo mode.

| Operation | Method and path | Replaces |
| --- | --- | --- |
| `listAdminLiveTVTuners` | GET `/api/v2/admin/livetv/tuners` | GET `/api/v1/livetv/tuners` |
| `addAdminLiveTVTuner` | POST `/api/v2/admin/livetv/tuners` | POST `/api/v1/livetv/tuners` |
| `discoverAdminLiveTVTuners` | POST `/api/v2/admin/livetv/tuners/discover` | POST `/api/v1/livetv/tuners/discover` |
| `deleteAdminLiveTVTuner` | DELETE `/api/v2/admin/livetv/tuners/{tuner_id}` | DELETE `/api/v1/livetv/tuners/{tunerId}` |
| `scanAdminLiveTVTuner` | POST `/api/v2/admin/livetv/tuners/{tuner_id}/scan` | POST `/api/v1/livetv/tuners/{tunerId}/scan` |
| `updateAdminLiveTVChannel` | PATCH `/api/v2/admin/livetv/channels/{channel_id}` | PATCH `/api/v1/livetv/channels/{channelId}` |
| `listAdminLiveTVGuideSources` | GET `/api/v2/admin/livetv/guide-sources` | GET `/api/v1/livetv/guide-sources` |
| `createAdminLiveTVGuideSource` | POST `/api/v2/admin/livetv/guide-sources` | POST `/api/v1/livetv/guide-sources` |
| `updateAdminLiveTVGuideSource` | PATCH `/api/v2/admin/livetv/guide-sources/{source_id}` | PATCH `/api/v1/livetv/guide-sources/{sourceId}` |
| `deleteAdminLiveTVGuideSource` | DELETE `/api/v2/admin/livetv/guide-sources/{source_id}` | DELETE `/api/v1/livetv/guide-sources/{sourceId}` |
| `syncAdminLiveTVGuideSource` | POST `/api/v2/admin/livetv/guide-sources/{source_id}/sync` | POST `/api/v1/livetv/guide-sources/{sourceId}/sync` |
| `lookupAdminLiveTVSchedulesDirectLineups` | POST `/api/v2/admin/livetv/schedules-direct/lineups` | POST `/api/v1/livetv/guide-sources/schedules-direct/lineups` |
| `lookupAdminLiveTVXMLSyncLineups` | POST `/api/v2/admin/livetv/xml-sync/lineups` | POST `/api/v1/livetv/guide-sources/xml-sync/lineups` |

Lists use `{items}` envelopes; discovery returns `{items, notes}`. Lineup
lookups take `postal_code` (v1: `postalcode`).

## Differences from v1

- Collections are `{items: [...]}` instead of named arrays.
- Session start is `.../sessions` (plural); the response drops `hls_url` in
  favour of `stream_url` and adds `status`, `created_at` and
  `stale_after_seconds`. Capabilities move under `client_capabilities`.
- Heartbeat and stop return `204`; v1 stop returned the session.
- Recordings omit `user_id` and the on-disk `path`.
- The guide takes repeated `channel_id` instead of comma-separated `channels`.
- Validation failures are `422` problems instead of `400`.
