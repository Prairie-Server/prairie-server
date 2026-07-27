# Live TV tuner discovery (HDHomeRun + Dispatcharr)

Prairie can discover Live TV tuners from **Admin → Live TV → Tuners**:

| Mode | What it finds | Requirements |
| --- | --- | --- |
| **Discover on LAN** | SiliconDust HDHomeRun (UDP port `65001`) and any Dispatcharr instance that answers HDHR UDP discovery | Host must be able to broadcast/receive on the LAN |
| **Probe URL only** | Dispatcharr HDHR emulation (`/hdhr/discover.json`) or a direct HDHR `discover.json` | Reachable HTTP(S) URL |

Candidates are verified over HTTP before you click **Add**. Existing tuners are marked **Added**.

Manual add takes a single **tuner URL** (base URL or host). Prairie probes the usual `discover.json` locations, including Dispatcharr’s `/hdhr/` path, then stores the device identity and base URL from the response. You do not need a separate discover URL or device ID.

## Docker: enable LAN UDP discovery

Default `docker-compose.yml` runs Prairie in **bridge** networking. Bridge mode typically **cannot** send SiliconDust discovery broadcasts to your LAN, so **Discover on LAN** returns no devices (URL probe still works).

### Linux: use the Live TV compose override

```sh
docker compose -f docker-compose.yml -f docker-compose.livetv.yml up -d
```

Or set in `.env`:

```dotenv
COMPOSE_FILE=docker-compose.yml:docker-compose.livetv.yml
```

What the override does:

- Sets `network_mode: host` on the `prairie` service
- Points `DATABASE_URL` / `REDIS_URL` at `127.0.0.1` (postgres/redis still publish host ports)
- Uses `PORT` from `.env` as the host listen port (default `8090`, same as the previous published URL)

You can combine with NVIDIA:

```sh
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml -f docker-compose.livetv.yml up -d
```

### Docker Desktop (macOS / Windows)

`network_mode: host` does **not** attach the container to your real LAN. Prefer:

1. **Probe URL** with your Dispatcharr base (`http://dispatcharr.local:9191`) or HDHR IP (`http://192.168.1.50`)
2. Or run Prairie **from source on the host** (not in Docker) if you need UDP discovery

### Bare metal / from source

No special config — UDP discovery uses the host network stack directly.

## Dispatcharr

Dispatcharr exposes HDHomeRun-compatible endpoints (commonly under `/hdhr/`). Prairie probes:

- `{base}/hdhr/discover.json`
- `{base}/discover.json`
- `http(s)://{host}:9191/hdhr/discover.json` when another port was given

No Dispatcharr API key is required for HDHR emulation; protect it with network ACLs as you would any tuner.

## API

`POST /api/v1/livetv/tuners` accepts:

```json
{ "url": "http://192.168.1.50" }
```

Legacy `discover_url` and `device_id` fields are still accepted as aliases for the same address. The SiliconDust hardware id is read from `discover.json` and stored as `device_id` on the tuner; clients do not supply it.

## Security notes

- Discovery and probe only accept `http://` / `https://` URLs (same Live TV fetch allowlist as manual add).
- Do not expose HDHR/Dispatcharr discover or stream URLs to the public internet.
- Admin-only API: `POST /api/v1/livetv/tuners/discover`
