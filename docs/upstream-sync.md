# Syncing upstream Silo into Prairie

Prairie is an AGPL fork of Silo. Upstream syncs are routine, and they are also
the main way Prairie loses work: a conflicted file resolved wholesale to
upstream deletes Prairie's hunks while their tests survive. Past losses include
the stream-token auth mount (TV playback 401s), Live TV web routes, the Tizen
HLS tag strip, the audio channel ceiling and the jellycompat Live TV wiring.

## Rules

- Sync on a `sync/upstream-<date>` branch with a real `git merge upstream/main`,
  and land it with **"Create a merge commit"**. Never squash; it drops upstream
  ancestry and the next sync re-conflicts everything.
- **A sync PR merges only with CI green.** A failing test after a sync usually
  means Prairie code was dropped while its test was kept, not that the test is
  stale.
- `Prairie invariants` (scripts/check-prairie-invariants.sh) must pass. If it
  fails, restore the code. Do not edit the manifest to make it pass.

## After resolving conflicts

1. Rewrite `github.com/Silo-Server/silo-server` to the Prairie module path
   across the tree (auto-merged upstream files bring Silo paths back), re-add
   Prairie-only imports, run goimports.
2. Hunk audit: for every file in `git diff --name-only <merge-base> <prairie-parent>`,
   confirm each Prairie hunk still applies in reverse to the merge result.
   Include web `.ts`/`.tsx`.
3. Every `observe*(…, http.MethodX, "pattern")` literal must appear in that
   package's `media_routes.go`. An undeclared route panics at startup and the
   pod sits Running/0-Ready with nothing logged.
4. Every page under `web/src/pages` must still be imported somewhere. An
   orphaned page means lost wiring.
5. Rebrand fallout: new upstream tests asserting "Silo" strings, `Silo/` in the
   User-Agent, `X-Silo-*` headers (Prairie uses `X-Prairie-*` and `prairie.*`
   wire names).
6. Run `pnpm exec tsc -b` for the web app. PR CI does not.

## When you restore something a sync dropped

Add an anchor for it to `scripts/prairie-invariants.txt` in the same PR:
the file, the minimum number of matches, a regex on a stable identifier, and
where it came from. That is what stops the next sync from deleting it again.
