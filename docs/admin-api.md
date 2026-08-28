# Admin API

Server-administration endpoints under `/api/v1/admin`. Every `/api/v1/admin`
route requires an authenticated account with the server-wide `admin` role — the
same authorization as `/api/v1/admin/sessions` — and none of them are part of
the client-facing contract that third-party apps build against. A few
deliberately public reads outside `/api/v1/admin` (marked `public` in the route
tables) are documented beside the admin writes they pair with.

This document is new and covers only the routes listed below. The rest of the
admin surface predates it and is currently documented by the code and by the
design documents under `docs/design/`.

## Branding assets

Uploadable images white-label the server: the sidebar wordmark, the square
mark (collapsed sidebar and installed PWA), optional light-theme variants of
both, the browser favicon, and the login background. Each is stored in the public S3 bucket and referenced from a
`server_settings` row, so uploads return `503 unavailable` until
`s3.public_bucket` is configured.

| Route                                         | Auth   | Purpose                                                              |
| --------------------------------------------- | ------ | -------------------------------------------------------------------- |
| `POST /api/v1/admin/branding/assets/{kind}`   | admin  | Upload (multipart, field name `file`). Replaces whatever is stored.  |
| `DELETE /api/v1/admin/branding/assets/{kind}` | admin  | Clear the asset. `204`, and clearing an unset asset is not an error. |
| `GET /api/v1/branding/assets/{kind}`          | public | Serve the stored bytes. Content-addressed, so `immutable` cached.    |
| `GET /api/v1/theme/branding`                  | public | Current branding, including each asset URL (omitted when unset).     |

Public reads are deliberately unauthenticated: branding has to apply on the
login page, before anyone has a session.

`{kind}` is one of `wordmark`, `wordmark_light`, `mark`, `mark_light`, `favicon`, `login_bg` — the light variants follow their base kind's processing. Uploads are
processed per kind — the numbers below are the contract the admin UI quotes back
to the operator, and they live in `internal/branding/assets.go`:

| Kind       | Accepts             | Max upload | Stored as                                                                                                     |
| ---------- | ------------------- | ---------- | ------------------------------------------------------------------------------------------------------------- |
| `wordmark` | PNG, JPEG, WebP     | 8 MB       | WebP, aspect preserved, capped at 640px wide. Narrower art is not enlarged.                                   |
| `mark`     | PNG, JPEG, WebP     | 8 MB       | WebP, center-cropped to a square, then forced to exactly 512×512 (smaller art is upscaled).                   |
| `favicon`  | PNG, WebP, ICO, SVG | 1 MB       | Byte-for-byte as uploaded, so `.ico` and `.svg` keep working in browsers that will not render a WebP favicon. |
| `login_bg` | PNG, JPEG, WebP     | 12 MB      | WebP, aspect preserved, capped at 2560px wide. Clients display it cover-cropped.                              |

There is one stored variant per kind, not a responsive set: the PWA manifest
advertises the single 512px mark at both 192×192 and 512×512, and native clients
read the same URLs as the web app. Recommend source art at or above the stored
size — anything larger is downscaled, anything smaller is either left small
(wordmark, login background) or upscaled (mark).

Failure modes: `400 bad_request` for an unknown kind, a missing `file` field, or
a content type the kind does not accept; `413 too_large` past the cap;
`503 unavailable` when asset storage is not configured.

Uploaded SVG favicons are admin-controlled but served from the app origin, so
every asset response carries `X-Content-Type-Options: nosniff` and a sandboxing
`Content-Security-Policy` — a directly-navigated SVG cannot run script in the
viewer's session.

## Server status and restarts

Some settings are only read at startup. Two routes carry that contract:

| Route | Auth | Purpose |
|---|---|---|
| `GET /api/v1/admin/server/status` | admin | Process start time and pending-restart state. |
| `GET /api/v1/admin/settings/restart-keys` | admin | The compiled registry of setting keys that only take effect after a restart (`internal/config/restart_keys.go`). |

`GET /api/v1/admin/server/status` response:

| Field | Type | Meaning |
|---|---|---|
| `started_at` | RFC3339 string | When this process started. |
| `restart_required` | bool | A restart-required change was saved. Latches true for the life of the process; a real restart clears it by starting a new process. |
| `restart_required_at` | RFC3339 string | When the flag first latched. Omitted until then. |
| `restart_required_reason` | string | The reason of the **last** restart-required save only — later saves overwrite it. |
| `restart_required_reasons` | string[] | Every distinct reason since boot, first-seen order. Settings saves record one `setting:<key>` entry per restart-required key, so a client can scope a pending restart to the subsystem it belongs to. |
| `restart_mark_count` | int | Increments on every restart-required save. Because the boolean latches, this counter is the only signal that a **new** requirement arrived — the admin UI re-arms its dismissed restart banner on it. |
| `restart_requested`, `restart_requested_at` | bool, RFC3339 string | An in-app restart was requested, and when. |

## `GET /api/v1/admin/stream-telemetry/parity`

Returns the merged stream-telemetry view beside the two legacy live-session
projections an admin reads today, plus the diff between them.

It is a diagnostic: it compares and does not cut over. No existing admin read has
been repointed onto telemetry, and nothing here blocks, throttles or ends a
session. Design: [`docs/design/2026-08-17-stream-telemetry.md`](design/2026-08-17-stream-telemetry.md).

The view is served from a bounded-staleness cache with single-flight refresh, so
several admins polling this route pay at most one rebuild per TTL.

Stream telemetry runs by default, so this route reports on an unconfigured
server. An `enabled: false` body means this process was switched off with
`SILO_STREAM_TELEMETRY_ENABLED=false`, or that a bad core setting disabled it —
the startup log names the variable in that case.

### Response

Always `200 OK`. "Nothing to compare" is expressed in the body rather than as an
error status, because an empty report with a success status would read as
agreement.

| Field     | Type   | Meaning                                                                              |
| --------- | ------ | ------------------------------------------------------------------------------------ |
| `enabled` | bool   | Stream telemetry is running in this process.                                         |
| `reason`  | string | Present when there is nothing to compare (telemetry disabled, or no view built yet). |
| `view`    | object | State of the merged view the comparison was built from.                              |
| `sources` | array  | One report per legacy projection. Empty when `enabled` is false.                     |

`view`:

| Field                                 | Type             | Meaning                                                                                                                                                                                            |
| ------------------------------------- | ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `available`                           | bool             | A merged view exists.                                                                                                                                                                              |
| `built_at`                            | RFC3339 string   | When it was built. Omitted if never.                                                                                                                                                               |
| `age_ms`, `stale`                     | int, bool        | Age of the cached view, and whether it exceeded the TTL.                                                                                                                                           |
| `build_took_ms`                       | int              | Cost of the last rebuild.                                                                                                                                                                          |
| `refreshes`, `failures`, `last_error` | int, int, string | Cache counters since process start.                                                                                                                                                                |
| `complete`                            | bool             | No publisher was stale, degraded or truncated.                                                                                                                                                     |
| `incomplete_reasons`                  | string[]         | Why `complete` is false — e.g. `missing_publisher`, `publisher_truncated`, `decode_errors`, `truncated`.                                                                                           |
| `missing_publishers`                  | string[]         | Publisher ids present in the roster but with no usable snapshot.                                                                                                                                   |
| `clock_skew_suspected`                | bool             | A publisher stamped a time in the future. A clock running _behind_ is indistinguishable from a stalled publisher in one sample; compare `publishers` sequence across two reads to tell them apart. |
| `publishers`                          | string[]         | `<publisher-id>=<state>`, where state is `fresh`, `degraded`, `stale` or `departed`.                                                                                                               |
| `session_count`, `transfer_count`     | int              | Sizes of the merged view.                                                                                                                                                                          |

Each entry in `sources`:

| Field       | Type     | Meaning                                      |
| ----------- | -------- | -------------------------------------------- |
| `source`    | string   | `playback_sessions_sync` or `node_sessions`. |
| `available` | bool     | The projection could be read.                |
| `error`     | string   | Why it could not.                            |
| `notes`     | string[] | Caveats that apply to this comparison.       |
| `report`    | object   | The diff, when available.                    |

`report`:

| Field                                               | Type     | Meaning                                                                                                                          |
| --------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `telemetry_count`, `legacy_count`, `in_both`        | int      | Session counts on each side and their intersection.                                                                              |
| `agrees`                                            | bool     | Same session set, and no field both sides express disagrees. Read `fields_absent` before treating this as clearance to cut over. |
| `telemetry_only`, `legacy_only`                     | string[] | Session ids present on one side only, capped.                                                                                    |
| `telemetry_only_truncated`, `legacy_only_truncated` | int      | How many ids the cap dropped.                                                                                                    |
| `mismatches`                                        | object[] | Per-session field disagreements, capped.                                                                                         |
| `mismatches_truncated`                              | int      | How many the cap dropped.                                                                                                        |
| `fields_absent`                                     | object   | Per field, sessions both sides know where one side carries no value. A gap in a projection, not a disagreement.                  |

A single report samples three independently updated stores, so one-sided
differences are normal and are not on their own evidence of a defect. Repeated
agreement over time is what the legacy-retirement project is gated on.
