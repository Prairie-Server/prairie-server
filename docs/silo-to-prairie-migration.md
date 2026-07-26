# Silo to Prairie migration

This Phase 1 rebrand changes names, paths, environment variables, Docker defaults, and search-index defaults from Silo to Prairie. It does not change the AGPL license or the plugin SDK module path (`github.com/prairie-server/prairie-plugin-sdk`).

## Paths

Move persistent host data from Silo paths to Prairie paths before starting the rebranded stack:

| Silo | Prairie |
| --- | --- |
| `/opt/silo` | `/opt/prairie` |
| `/var/lib/silo` | `/var/lib/prairie` |
| `/tmp/silo-transcode` | `/tmp/prairie-transcode` |
| `silo-download-artifacts` | `prairie-download-artifacts` |

For Docker installs, set `PRAIRIE_DATA_ROOT=/opt/prairie` and update bind mounts accordingly. If you keep data in a custom location, rename the environment variable even if the underlying path stays the same.

## Environment variables

Rename `SILO_*` variables to `PRAIRIE_*`. Runtime code accepts legacy `SILO_*` fallbacks for direct env reads where this phase touched the reader, but new configuration should use Prairie names. Common examples:

| Old | New |
| --- | --- |
| `SILO_DATA_ROOT` | `PRAIRIE_DATA_ROOT` |
| `SILO_IMAGE` | `PRAIRIE_IMAGE` |
| `SILO_PUBLIC_URL` | `PRAIRIE_PUBLIC_URL` |
| `SILO_PLUGIN_CACHE_DIR` | `PRAIRIE_PLUGIN_CACHE_DIR` |
| `SILO_TRUSTED_PROXIES` | `PRAIRIE_TRUSTED_PROXIES` |
| `SILO_MIGRATE_TIMEOUT` | `PRAIRIE_MIGRATE_TIMEOUT` |
| `SILO_OTEL_ENABLED` | `PRAIRIE_OTEL_ENABLED` |
| `SILO_TEST_DATABASE_URL` | `PRAIRIE_TEST_DATABASE_URL` |

The agent/debug helper file is now `.prairie-dev.env`, and the script is `scripts/prairie-dev`.

## Docker Compose

The main application service is now `prairie`, and the default image is:

```dotenv
PRAIRIE_IMAGE=ghcr.io/prairie-server/prairie-server:latest
```

The bundled PostgreSQL defaults are now:

```dotenv
POSTGRES_USER=prairie
POSTGRES_PASSWORD=prairie
POSTGRES_DB=prairie
```

If you are migrating an existing database, keep the old PostgreSQL username/database until you intentionally rename them or create a new database and restore into it. The application only requires that `DATABASE_URL` points at the correct database.

## PostgreSQL cutover

For a bundled compose install that keeps the same database volume, the conservative cutover is:

1. Stop the Silo stack.
2. Back up PostgreSQL and the old `/opt/silo` tree.
3. Move or copy `/opt/silo` to `/opt/prairie`.
4. Update `.env` to use `PRAIRIE_*` names.
5. If preserving the existing database/user names, set `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` to the old values. If creating a fresh Prairie database, restore the dump into the new `prairie` database.
6. Start the Prairie stack and verify `/api/v1/health`.

## Meilisearch cutover

Default Meilisearch settings now use:

| Old | New |
| --- | --- |
| `silo_media_items` | `prairie_media_items` |
| `silo_recommendations` | `prairie_recommendations` |

Existing indexes can be left in place during cutover, but Prairie defaults will create/use the new names for fresh settings. To avoid stale search results, rebuild the catalog search index after changing the index or embedder name.

## Headers and clients

Prairie emits `X-Prairie-*` headers. The server accepts legacy `X-Silo-*` request headers for the rebranded header set during this phase, so older clients can continue to connect while native clients switch over.
