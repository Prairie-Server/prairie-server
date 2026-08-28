# Prairie

Prairie is a self-hosted media streaming server for movies, shows, music, and books. It is an AGPL-licensed fork of Silo, rebranded with its own project identity while preserving the same core goal: point Prairie at your media folders and stream to your devices at home or away with direct play, remuxing, and hardware-accelerated transcoding handled automatically.

## Highlights

- **Plays your media, your way** — direct play when the device supports it, remux or hardware-accelerated transcode when it does not.
- **Web app included** — a full-featured web client and admin interface ship with the server.
- **Jellyfin-compatible surface** — optional compatibility APIs support clients such as VidHub, Findroid, and Infuse.
- **Household profiles** — multiple profiles per account, with per-profile watch state and parental controls.
- **Plugin-driven metadata** — match and enrich libraries with providers like TMDB and TVDB through plugins.
- **Fast setup** — one `docker compose up -d` starts the default stack; most configuration happens in the admin UI.

## Deploy with Docker

1. Copy the example environment file:

   ```sh
   cp .env.example .env
   ```

2. Edit `.env` and set at least your media path:

   ```dotenv
   MEDIA_ROOT=/path/to/your/media
   ```

   New Prairie installs default persistent bind mounts to `/opt/prairie`, with the app's internal paths under `/var/lib/prairie` and transient transcodes under `/tmp/prairie-transcode`. You can override the host root with `PRAIRIE_DATA_ROOT`.

3. Start the integrated stack:

   ```sh
   docker compose up -d
   ```

   This starts PostgreSQL, Redis, and the integrated Prairie server. The web app is available at `http://localhost:8090`.

The default PostgreSQL user and database are `prairie`. The published image defaults to `ghcr.io/prairie-server/prairie-server:latest`.

### Optional NVIDIA/NVENC

GPU support is kept in a compose override so hosts without NVIDIA drivers work unchanged:

```sh
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d
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

See [docs/silo-to-prairie-migration.md](docs/silo-to-prairie-migration.md) for the Phase 1 path, environment variable, PostgreSQL, and Meilisearch cutover checklist. Legacy `SILO_*` runtime environment variables are accepted as fallbacks where the server reads them directly, but new installs should use `PRAIRIE_*`.

Existing Continuum-to-Silo migration notes remain in [docs/continuum-to-silo-docker-migration.md](docs/continuum-to-silo-docker-migration.md) for historical installs that need that earlier hop.

## Build from source

Prerequisites: Go 1.26+, pnpm, PostgreSQL, Redis, and FFmpeg.

```sh
cp .env.example .env
make build
./prairie
```

The server listens on `http://localhost:8080` by default when run from source.

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
