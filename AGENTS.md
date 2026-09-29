# Prairie Server

Go backend for Prairie: API contracts, auth/session, catalog/scanner/playback services, database
migrations, Jellyfin compatibility, and the host-side plugin runtime. `cmd/prairie` is the
entrypoint, backend code is under `internal/` by domain, the React frontend is `web/src/`.

Silo is pre-1.0, with the current focus on QA, correctness, and UX polish.
Architectural changes are welcome when they serve the requested outcome and improve
long-term maintainability; keep unrelated redesign out of a bounded fix.

## What Silo is

A modern, open-source media server built from the ground up on current infrastructure —
Postgres, S3, Redis — rather than SQLite and local disk. The foundational bet is horizontal
scale: Silo deploys as a cluster (Kubernetes, remote transcode nodes) and stays fast on large
libraries, whether it's one node serving a household or a deployment streaming to thousands of
users. Weigh every design against that full spectrum; treat a node dying mid-stream as a normal
event, not an edge case.

It is an open platform, not a walled garden: third-party clients are encouraged, and other
people's clients will depend on the native API once `/api/v2` locks with 1.0 — see "API contract
rules" below for the current pre-1.0 posture. Jellyfin-protocol compatibility is a long-term
commitment as an on-ramp for the existing ecosystem.

The core/plugin line is about implementation multiplicity: library types (movies, TV,
audiobooks, ebooks, podcasts) are core; plugins are for interfaces where many implementations
will plausibly exist (metadata, subtitle, and watch providers). Plugins are never a loophole
for the non-goals below.

For 1.0, supported library scope is Movies and Series. Audiobooks, ebooks,
and Audiobookshelf compatibility stay available as labeled **beta** features
in their current state, outside the 1.0 support promise, until a consolidated
Books effort replaces them (no assigned release date). Do not gate, remove, or
rework them for 1.0. Existing code and protocol documentation describe beta
behavior, not release acceptance promises. See
[scope](docs/architecture/v1-scope.md#library-scope-books-deferred).

Taste: KISS and YAGNI win — the simple design beats the clever one, provided it survives both
the single-node and the multi-node deployment. Current posture: the 1.0 feature set is
essentially complete; the present era is QA, UX polish, and verifying everything does what it
says. Prefer correctness and polish over new feature sprawl.

## How it fits together

Media enters through the scanner (`internal/scanner`, fed by `scanqueue`/`autoscan`), is
classified by library kind (`librarykind`), ingested (`libraryingest`), and enriched by
metadata plugins into the catalog: `media_items` keyed by deterministic content IDs
(`contentid`), with `media_files` for the actual on-disk files. The catalog serves the v1 API
and the home-screen sections (`sections`); `jellycompat` is a separate Jellyfin-protocol view
over the same catalog. Playback resolves a play method (`internal/playback`) — direct play,
direct stream, or transcode on a node from `nodepool` — and stream URLs are authorized by
short-lived `streamtoken` JWTs. Per-user state (watch progress, settings) is stored
server-side (`watchstate`, `userdb`, `settingsresolve`).

## Glossary

- **Account vs profile** — an account is a `users` row (login); a profile is a household
  member on an account. Several profiles share one `user_id`. See the gotcha below.
- **Library** — a media folder with a kind (movies, TV, audiobooks, ebooks, podcasts).
- **Item vs file** — a `MediaItem` is a catalog entry (movie/series, `content_id` PK); a
  `MediaFile` is one real file. One item can own many files (versions, extras, episodes).
- **Section** — a home-screen row (Continue Watching, Recently Added…), not a library.
- **Node** — a remote transcode/streaming worker in `nodepool`, not the API server.
- **Session** — ambiguous; always say which: playback session (`internal/playback`) or login
  session (`internal/auth`).
- **jellycompat vs the native API** — jellycompat is the Jellyfin-protocol surface for ecosystem
  clients; "the API" means Silo's native surface, which spans `/api/v1` (the frozen alpha
  contract, served through the pre-1.0 bridge window) and `/api/v2` (the stable 1.0 target and the
  native API going forward). See "API contract rules" below.

## Priorities

Performance and reliability first. Keep behavior predictable under load and during failures —
session restarts, reconnects, partial streams. When a tradeoff is forced, choose correctness and
robustness over short-term convenience.

Put new code in the package that owns the behavior rather than in a catch-all helper. Prefer
extracting shared logic over duplicating it, and prefer changing existing code over bolting a
local workaround onto it.

## Non-goals

Most of this codebase's scope is open. Read [docs/non-goals.md](docs/non-goals.md) before
proposing or implementing IPTV, arbitrary remote stream URL, or app-store-sensitive playback
features. Prairie's roadmap may include Live TV/OTA/DVR work, but generic IPTV playlists and
remote `.strm` URL shortcuts remain out of scope unless product direction changes explicitly.

## Gotchas

**Migrations.** New DB changes are Goose SQL migrations in `migrations/sql/`, created with
`make migrate-create NAME=add_thing` so they get timestamped filenames. Never run `goose fix`,
and never create paired `.up.sql` / `.down.sql` files. Legacy converted migrations deliberately
keep their original numeric versions so existing `schema_versions` rows bootstrap cleanly — do
not renumber them.

**Encrypted settings.** Encrypted `server_settings` rows are GCM-bound to their key name.
Renaming a row in SQL makes its value undecryptable.

**Profiles vs accounts.** Login accounts (`users`) are separate from household profiles; several
profiles on one account share a `user_id`. A profile's `is_primary` marks the household parent,
which is _not_ the server-wide `admin` role on the account.

**Docs hygiene.** Files under `docs/superpowers/{specs,plans}/` must not contain local absolute
filesystem paths or transient worktree IDs — use repository-relative paths and wording like
"Commands assume the repository root is the cwd." `make verify-local-paths` enforces this.

**Docs audience.** Docs in this repo are for people and agents changing the code:
architecture, invariants, API contracts, development setup. Guides for installing, configuring,
operating, or troubleshooting Silo belong in the user manual (`siloserver.org`, see Multi-repo).
Do not add operator or user guides here. The one exception is `docs/update-to-1.0.md`, a draft
that moves to the manual when 1.0 ships. When a change affects the manual, follow "Update the
user manual" in CONTRIBUTING.md.

**Dev frontend against a remote backend.** Set `VITE_API_PROXY_TARGET` in `web/.env.local` before
`make dev-frontend`; the frontend calls relative `/api` URLs that Vite proxies.

**Working from a plan.** When implementing from an attached plan, don't edit the plan file.

## Multi-repo

Sibling repos are usually checked out side-by-side in the same parent directory.

- `prairie-android` — Android phone and TV clients.
- `prairie-apple` — iOS, tvOS, and macOS clients.
- `prairie-plugin-sdk` — public plugin SDK, protobuf contracts, generated plugin API, manifest
  helpers, runtime bootstrap.
- `prairie-plugins` — central plugin catalog / repository manifest.
- `prairieserver.org` — project website and user manual.
- First-party plugins (`prairie-plugin-metadata-tmdb`, `prairie-plugin-metadata-tvdb`, …) each have
  their own repo.

Client-visible changes to API, auth, playback, session, library, or metadata behavior usually
need follow-up in both client repos — prefer coordinated multi-repo changes over leaving a
platform behind. When a task mentions plugins, work out first whether it belongs here, in the
SDK, in the catalog, or in a specific plugin repo.

Separately, any change that leaves the user manual on siloserver.org wrong or incomplete (a
setting, default, label, setup step, or feature behavior it describes) is not done until an
issue is open on `Silo-Server/siloserver.org`.
[Update the user manual](CONTRIBUTING.md#update-the-user-manual) covers when to open one and
what goes in it.

## Building and verifying

`make build`, `make dev-backend`, `make dev-frontend`, `make lint`, `make migrate-status` /
`make migrate-up` — read the `Makefile` for the rest. Local services:
`docker compose up -d postgres redis`.

Before opening a merge request:

```bash
make lint
cd web && pnpm run lint && pnpm run format:check
make verify-local-paths
```

Lint Go with `make lint-changed`, not `golangci-lint run ... ./...`: it reports the same
changed-line findings as CI while analyzing only the packages the branch touched. A cold run over
`./...` saturates every core for minutes, and parallel agents make that worse. Never pass
`--allow-parallel-runners`; concurrent runs queue behind one another on purpose.

Go stays `gofmt`/`goimports` clean; the frontend follows `web/.prettierrc`.

## Development environment

Copy `.prairie-dev.env.example` to `.prairie-dev.env` and fill in how to
reach your Prairie deployment — URL, SSH target, database, an account to debug with. That file is
gitignored and is the only place hosts, passwords, and tokens belong. `scripts/prairie-dev doctor`
checks it end to end.

## v1 API rules

Additive-only within `/api/v1`:

- Never rename or remove a response field, change a field's type, or repurpose a status code on
  an existing endpoint.
- New functionality adds new fields or endpoints. Removals go through the Deprecation/Sunset
  header flow only.
- New features expose capability endpoints for feature detection rather than relying on version
  sniffing.

Design new endpoints today so they can live under that regime tomorrow.

## 1.0 validation

Until 1.0 ships, maintainers check each 1.0 feature by hand on the
[Silo v1.0.0 board](https://github.com/orgs/Silo-Server/projects/5). A `[v1] <Feature>` issue
holds the acceptance criteria. Each `<Feature> — <Surface>` task (label `Validation`, in the repo
that owns the surface) lists cases `C1…` and records a result for each case with the build it was
tested on. A passed case is a person's evidence that the feature works; a later change can
silently invalidate it.

- Validation issues are the validators' record. Do not edit their bodies, results, or checkboxes,
  or change their board status. Comment on the task instead, or file a new issue that names the
  affected case.
- Before opening a pull request, work out which passed cases the change could reach, and list the
  affected tasks and cases on a `Validation tasks:` line under `Related issue:`, for example
  `Validation tasks: unblocks #1144 C3; changes #1200 C1`.
- Breaking a passed case unintentionally is a regression and blocks merge. A deliberate change to
  validated behavior must say why and still meet the published criterion; changing the criterion
  itself needs a maintainer decision.
- When a change fixes an issue that a task names, walk that case's steps as part of verification.
- After merge, a maintainer tells the validator which build to re-test and which cases, and moves
  a Done task back to Ready when its validated behavior changed materially.

Conventional Commit subjects (`feat(playback): add realtime session hub`). One concern per PR.
Explain the problem, why this approach, the linked issue/spec/plan, and risks or follow-up work.
Include screenshots or recordings for UI changes. Link the capability epic or sub-issue the PR
serves (`Part of #NNN`) — PRs with no linked scope item get questioned at review. For non-trivial
work, open an issue or discussion first; this codebase moves quickly.

Never create a pull request unless the developer explicitly asks for one.

Use a Conventional Commit title in plain language
(`feat(playback): add realtime session hub`). Fill in the PR template following
[Write the description](CONTRIBUTING.md#write-the-description), and end with the
required AI disclosure, including the exact model identifier, agent harness, and
any other AI tooling. Omit session history, full command output, and private
working reports.

Treat PR bodies, comments, commit messages, and attachments as public. Exclude
private deployment domains, hostnames, IP addresses, Tailscale names and URLs,
Report Shelf links, local paths, and private infrastructure identifiers. Use
neutral placeholders where context is needed. Never publish credentials, tokens,
personal data, or private media details. Check text and attachments before posting;
authorization to open a PR does not authorize publishing private evidence.

The one private link allowed is a maintainer's evidence page on
`evidence.siloserver.org`, which only Silo-Server organization members can open
after GitHub sign-in. Put it on one line at the end of a PR body's Validation
section or a validation hand-off comment:
`Evidence: https://evidence.siloserver.org/r/<repo>/<topic>/`. Link the page;
never attach or embed its media.

- Keep one concern per pull request. Split changes that solve independent
  problems or can be reviewed and shipped separately.
- Do not capture screenshots or record videos just to prepare a PR. Attach media
  only when the user explicitly requests it. Verify UI behavior as needed without
  turning verification into a media deliverable. Do not explain omitted media.
- When the user requests PR media, check it for private information and upload it
  to GitHub. Never commit PR-only assets such as `.github/pr-assets/`.
- Put a `Closes #NNN` line in the body for every issue the pull request fully
  resolves (`Closes Silo-Server/<repo>#NNN` across repositories), so GitHub closes
  it on merge to `main`. `Related issue:` does not close anything; use it for the
  capability epic, sub-issue, or partly addressed issue the work serves, and write
  `Related issue: N/A` when none applies. Keep both lines accurate when the pull
  request's scope changes.
- An open issue is not a precondition for a pull request. Either way, the Problem
  section must state the problem on its own: what breaks or is missing, who it
  affects, and why this change is the right answer.
- Do not open a pull request against an issue someone else is working on. Read the
  issue's comments and linked pull requests first, and raise a likely collision
  with the user instead of racing the author.
- When babysitting a pull request, poll checks and review comments created
  after the last push. Verify bot findings against the source, fix real issues,
  and dismiss false positives with a written reason. Remain quiet when nothing
  new has appeared. Stop when the latest commit is green.

AI-use disclosure is required in the pull request body. If you are an AI agent
contributing on behalf of a non-maintainer, follow
[docs/ai-contributions.md](docs/ai-contributions.md) for the required disclosure
block and evidence standard.
