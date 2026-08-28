# Renovate presets

Shared [Renovate](https://docs.renovatebot.com/) configuration for Prairie-Server repositories.

These live in `prairie-server` because the org `.github` repository is not currently writable from automation; they can move to `Prairie-Server/.github` later (`renovate-config.json`) without changing behavior much.

| Preset | Extend as | Use for |
| --- | --- | --- |
| [`default.json`](./default.json) | `github>prairie-server/prairie-server//renovate-presets/default` | Base schedule, grouping, labels |
| [`actions-only.json`](./actions-only.json) | `github>prairie-server/prairie-server//renovate-presets/actions-only` | Plugin repos, themes, catalog |
| [`go-service.json`](./go-service.json) | `github>prairie-server/prairie-server//renovate-presets/go-service` | `prairie-plugin-sdk` and similar Go libraries |

Merge this directory to `main` before relying on sibling-repo configs that extend these presets.

## Forks

Most Prairie-Server repos are still GitHub forks of the old Silo network. With Mend installed on **All repositories**, Renovate skips forks unless the repo root `renovate.json` sets `"forkProcessing": "enabled"` (preset-only is not enough for the pre-clone check).
