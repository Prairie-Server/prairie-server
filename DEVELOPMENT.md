# Developing Prairie

This document covers building, running, and contributing to the Prairie server. If you just want to run Prairie, see the [README](README.md).

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations, merge request guidance, and the policy for AI-assisted submissions.

## Prerequisites

- Git, Make, and OpenSSL
- Docker Engine or Docker Desktop with Docker Compose 2.24+ (local services and testcontainers)
- Go 1.26.4+
- Node.js 24+ with pnpm 10.32.1. The managed Jellyfin Web installer (`make jellyfin-web`,
  `prairie compat-web install`, or the admin settings) also needs Node.js 24+ for Jellyfin Web 12.x.
- PostgreSQL 18 with pgvector
- Redis
- FFmpeg (transcoding). On macOS, install Homebrew's keg-only full build so
  text-subtitle burn-in and the complete filter set are available alongside
  VideoToolbox: `brew install ffmpeg-full`. Prairie discovers the Apple Silicon
  and Intel keg paths automatically when the FFmpeg Path setting is blank; a
  custom build can be selected explicitly in Admin Settings.
- A C compiler and build toolchain (CGO dependencies)
- pkg-config and the libvips development headers (image processing through bimg)

## Local Development

Source builds use only the PostgreSQL and Redis services of
[dev/compose.yml](dev/compose.yml); the README's `docker-compose.yml`
runs the published omnibus image instead.

```sh
# Start local PostgreSQL and Redis
docker compose -f dev/compose.yml up -d postgres redis

# Run the frontend dev server (hot reload, proxies API to :8090)
make dev-frontend

# Run the Go backend
make dev-backend
```

`dev/compose.yml` runs as the `prairie-dev` Compose project with named
volumes, so it does not reuse a development database that the earlier
`docker compose up -d postgres redis` kept under `PRAIRIE_DATA_ROOT/postgres`.
Stop those containers first (`docker compose down` with the earlier file), since
they hold ports 5432 and 6379, and restore a `pg_dump` of the old database into
the new one if you need its data.

If you are developing Prairie and `prairie-plugin-sdk` together, keep using the local [`go.work`](go.work) workspace. That workspace is a developer convenience only. CI and release builds run with `GOWORK=off`, so any new SDK helper used here must be pushed and tagged in `prairie-plugin-sdk` before this repo can merge or release the change.

Plugin authors should start with [docs/architecture/plugin-development.md](docs/architecture/plugin-development.md), which covers the RPC plugin package format, generated proto workflow, SDK import paths, route and asset exposure, and auth or user-config integration points.

## Make Targets

| Target                               | Description                                                         |
| ------------------------------------ | ------------------------------------------------------------------- |
| `make build`                         | Build frontend + Go binary                                          |
| `make frontend`                      | Build frontend only                                                 |
| `make dev-frontend`                  | Vite dev server with HMR                                            |
| `make dev-backend`                   | Run Go backend (integrated mode)                                    |
| `make dev-proxy`                     | Run a standalone proxy node                                         |
| `make dev-transcode`                 | Run a standalone transcode node                                     |
| `make migrate-create NAME=add_thing` | Create a timestamped Goose SQL migration                            |
| `make migrate-validate`              | Validate Goose migration files without touching a database          |
| `make migrate-status`                | Show Goose migration status using Prairie's bootstrapping runner    |
| `make migrate-up`                    | Apply pending Goose migrations using Prairie's bootstrapping runner |
| `make clean`                         | Remove build artifacts                                              |

## Database Migrations

PostgreSQL schema migrations are managed by Goose. Migration SQL files live in
`migrations/sql/` and use Goose annotations. Converted legacy migrations keep
their original numeric versions so existing `schema_versions` rows can bootstrap
cleanly into Goose without replaying old SQL. New migrations should be created
with timestamped filenames:

```sh
make migrate-create NAME=add_thing
make migrate-validate
```

Do not run `goose fix`; timestamped migrations are the repository policy because
they avoid version collisions across parallel PRs. The existing `001`-style
files are historical compatibility records, not the naming pattern for new work.
Runtime migrations are applied by the integrated/API server only. Proxy and
transcode modes never mutate schema.
For existing installs, use `make migrate-status` and `make migrate-up` rather
than invoking the Goose CLI directly; those targets copy legacy
`schema_versions` rows into `public.goose_db_version` under the migration lock
before reading or applying migrations. Set `ENV_FILE=path/to/.env` when the
database URL should be read from a non-default env file.

### Keep migrations online

Migrations run at startup while other API replicas keep serving, against tables
that can hold millions of rows. Keep locks on existing tables short:

- Create and drop indexes on existing tables with `CONCURRENTLY`, in a file
  marked `-- +goose NO TRANSACTION`. Run `DROP INDEX CONCURRENTLY IF EXISTS`
  before creating a new index: a copy left by an interrupted run is invalid, and
  `CREATE INDEX CONCURRENTLY IF NOT EXISTS` would keep it, so the planner would
  never use the index.
- When a migration replaces one index with another, a retry must not drop the
  replacement: goose records the version only after the last statement, so a
  run that already dropped the old index can run again. Use a Go migration
  that builds through `ensureIndexConcurrently` in `internal/database`, which
  drops only an invalid copy (`internal/database/push_attempt_key_migration.go`).
- Add foreign keys and `CHECK` constraints to existing tables as `NOT VALID`,
  and run `VALIDATE CONSTRAINT` only after that commits: in a later migration,
  or as a separate statement in a `NO TRANSACTION` file. Inside one
  transaction, validation keeps the lock the `ADD` took. A `CHECK` attached to
  `ADD COLUMN` scans the whole table under an `ACCESS EXCLUSIVE` lock.
- Backfill large tables in batches that commit as they go, through a procedure
  or a Go migration, rather than one statement over every row.
- Scope session settings to the transaction with `SET LOCAL` or
  `set_config(..., true)`. Compute partition bounds and dates in UTC, not in the
  server's `TimeZone`.

`20260914145000_index_artwork_reference_paths.sql` shows the index pattern, and
`20260930130041_store_media_search_fields.sql` the batched backfill.

## Running Tests

```sh
# Go tests (uses testcontainers — Docker must be running)
go test ./...

# Frontend tests
cd web && bun test
```

## Linting

```sh
# Go
golangci-lint run

# Frontend
cd web && bun run lint
cd web && bun run format:check
```

## Project Structure

```
cmd/prairie/       Entry point
internal/
  api/               HTTP router, handlers, middleware
  auth/              JWT authentication and sessions
  catalog/           Media item, episode, season repositories
  config/            YAML + env var configuration
  jellycompat/       Jellyfin/Emby protocol compatibility
  metadata/          Plugin-driven metadata matching and enrichment
  playback/          Direct play, remux, transcode session management
  scanner/           Media file discovery and FFProbe
  worker/            Background jobs (scan, match, reconcile)
web/                 React + TypeScript frontend (Vite, Tailwind, shadcn/ui)
migrations/sql/      Goose-managed PostgreSQL schema migrations
```
