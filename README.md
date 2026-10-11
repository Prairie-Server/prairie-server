<p align="center">
  <img src="assets/icon.png" alt="Prairie logo" width="112" height="112">
</p>

<h1 align="center">Prairie</h1>

<p align="center">
  A self-hosted media server for films, series, audiobooks, ebooks, podcasts, and manga.
</p>

<p align="center">
  <a href="https://github.com/Prairie-Server/prairie-server/releases"><img alt="Latest GitHub release" src="https://img.shields.io/github/v/release/Prairie-Server/prairie-server?include_prereleases&amp;sort=semver&amp;display_name=tag&amp;style=flat-square&amp;label=release"></a>
  <a href="https://github.com/orgs/Prairie-Server/packages/container/package/prairie-server"><img alt="Container image on GHCR" src="https://img.shields.io/badge/container-GHCR-2496ED?style=flat-square&amp;logo=docker&amp;logoColor=white"></a>
  <a href="https://github.com/Prairie-Server/prairie-server/actions/workflows/ci.yml"><img alt="Continuous integration" src="https://img.shields.io/github/actions/workflow/status/Prairie-Server/prairie-server/ci.yml?branch=main&amp;style=flat-square&amp;label=CI"></a>
  <img alt="Go 1.26" src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&amp;logo=go&amp;logoColor=white">
  <img alt="React 19" src="https://img.shields.io/badge/React-19-149ECA?style=flat-square&amp;logo=react&amp;logoColor=white">
  <a href="LICENSE"><img alt="AGPL-3.0-or-later license" src="https://img.shields.io/badge/license-AGPL--3.0--or--later-555555?style=flat-square"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick start</a>
  · <a href="https://prairieserver.org/docs">Documentation</a>
  · <a href="docs/release-versioning.md">Builds &amp; releases</a>
  · <a href="https://discord.gg/prairieserver">Discord</a>
  · <a href="#supporting-prairie">Support Prairie</a>
  · <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

> [!WARNING]
> Prairie is pre-release. APIs, configuration, and database migrations may change
> before the first stable release. Back up your deployment before updating.

## 1.0 release scope

The [1.0 milestone](https://prairieserver.org/milestone/1.0/) covers movies and
series. Audiobooks, ebooks, and Audiobookshelf compatibility keep working as
they do today and are labeled **beta**: they are outside the 1.0 support
promise and will be replaced by a consolidated Books effort with no assigned
release date. The broader capabilities below include those beta
implementations, not a promise that every media type is supported in 1.0.
Existing library data is preserved when upgrading.

## What Prairie does

<table>
  <tr>
    <td width="33%" valign="top">
      <strong>Play</strong><br><br>
      Direct play when possible, remux when needed, transcode otherwise, with
      VA-API, Quick Sync, and NVENC hardware acceleration.
    </td>
    <td width="33%" valign="top">
      <strong>Organize</strong><br><br>
      One catalog for films, series, audiobooks, ebooks, podcasts, and manga,
      matched through metadata plugins such as TMDB and TVDB.
    </td>
    <td width="33%" valign="top">
      <strong>Connect</strong><br><br>
      Use the included web app, or the Jellyfin/Emby-compatible API with clients
      such as <a href="https://vidhub.okaapps.com/what-does-vidhub-do/">VidHub</a>,
      <a href="https://github.com/jarnedemeulemeester/findroid">Findroid</a>, and
      <a href="https://firecore.com/infuse">Infuse</a>. Client coverage varies.
    </td>
  </tr>
  <tr>
    <td width="33%" valign="top">
      <strong>Share</strong><br><br>
      Household profiles with their own watch state, library access, and
      parental controls.
    </td>
    <td width="33%" valign="top">
      <strong>Manage</strong><br><br>
      Libraries, users, providers, storage, search, and playback are configured
      in the admin interface, not in config files.
    </td>
    <td width="33%" valign="top">
      <strong>Scale</strong><br><br>
      Start with one integrated server; split proxy and transcode roles across
      shared PostgreSQL and Redis when you need to.
    </td>
  </tr>
</table>

## Quick start

Requires Docker Compose 2.24 or newer. The default stack runs Prairie, PostgreSQL
with pgvector, and Redis.

```sh
git clone https://github.com/Prairie-Server/prairie-server.git
cd prairie-server
cp .env.example .env
chmod 600 .env
printf '\nPOSTGRES_PASSWORD=%s\nSECRET_KEY=%s\n' \
  "$(openssl rand -hex 24)" "$(openssl rand -base64 48)" >> .env
```

Or set `COMPOSE_FILE=docker-compose.yml:docker-compose.nvidia.yml` in `.env` on Linux/macOS (`;` instead of `:` on Windows).

### Optional Live TV LAN discovery (HDHomeRun)

Admin **Live TV → Tuners → Discover on LAN** uses SiliconDust UDP broadcasts (port `65001`). Bridge-mode Docker usually blocks that path; Dispatcharr / HDHR **URL probe** still works without changes.

On **Linux**, enable host networking for the Prairie service:

```sh
docker compose -f docker-compose.yml -f docker-compose.livetv.yml up -d
```

Or set `COMPOSE_FILE=docker-compose.yml:docker-compose.livetv.yml` in `.env`.

Details, Docker Desktop limits, and Dispatcharr probing: [docs/livetv-tuner-discovery.md](docs/livetv-tuner-discovery.md).

## Migrating from Silo

See [docs/silo-to-prairie-migration.md](docs/silo-to-prairie-migration.md) for the Phase 1 path, environment variable, PostgreSQL, and Meilisearch cutover checklist. Every `SILO_*` environment variable is also readable as `PRAIRIE_*`, and the `PRAIRIE_*` spelling wins when both are set; new installs should use `PRAIRIE_*`.

Existing Continuum-to-Silo migration notes remain in [docs/continuum-to-silo-docker-migration.md](docs/continuum-to-silo-docker-migration.md) for historical installs that need that earlier hop.

## Build from source

Prerequisites: Go 1.26+, pnpm, PostgreSQL, Redis, and FFmpeg.

Questions and discussion: [Discord](https://discord.gg/prairieserver).
Bugs, install problems, and performance issues: the
[GitHub issue forms](https://github.com/Prairie-Server/prairie-server/issues/new/choose),
which ask for reproduction steps and raw logs.

Native clients are developed in
[`prairie-apple`](https://github.com/prairie-server/prairie-apple) and
[`prairie-android`](https://github.com/prairie-server/prairie-android). Client-visible
API, authentication, playback, or metadata changes should consider both.

Right now, the most helpful way to contribute is a clear, accurate issue. We
implement and review changes through our own workflow, and pull request volume
has outgrown what we can review carefully, so a good issue usually helps us
more than a pull request.

Pull requests are still welcome from contributors who've had one merged in a
Prairie repository, or when a maintainer asks for one. We close other pull
requests without review. [CONTRIBUTING.md](CONTRIBUTING.md) explains what makes
an issue useful.

Useful development commands:

```sh
make dev-backend
make dev-frontend
make lint
make verify-local-paths
```

## Contributing

Prairie is free software under the GNU Affero General Public License v3.0 or later. Contributions should preserve the AGPL notices and the web UI's Source link. See [DEVELOPMENT.md](DEVELOPMENT.md), [CONTRIBUTING.md](CONTRIBUTING.md), and [docs/ai-contributions.md](docs/ai-contributions.md) for local workflow and contribution expectations.

## Trademark and attribution

Prairie is a fork of Silo. Silo names, logos, and wordmarks remain trademarks of their owners and are used only referentially. Prairie uses its own name and brand assets; see [TRADEMARK.md](TRADEMARK.md).

The source code is licensed under **AGPL-3.0-or-later**; see [LICENSE](LICENSE).
