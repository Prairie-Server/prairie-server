#!/usr/bin/env bash
# Fails when Prairie-only code listed in scripts/prairie-invariants.txt is gone.
# Upstream syncs have repeatedly resolved files wholesale to upstream and
# deleted Prairie wiring while keeping its tests; this is a cheap, build-free
# tripwire that runs on every PR. See docs/upstream-sync.md.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
manifest=scripts/prairie-invariants.txt
failures=0
checked=0
while IFS=$'\t' read -r path min pattern why || [[ -n "${path:-}" ]]; do
  [[ -z "${path// }" || "$path" == \#* ]] && continue
  checked=$((checked + 1))
  if [[ ! -f "$path" ]]; then
    echo "::error file=$path::Prairie invariant: file missing ($why)"
    failures=$((failures + 1))
    continue
  fi
  count=$(grep -cE -- "$pattern" "$path" || true)
  if (( count < min )); then
    echo "::error file=$path::Prairie invariant: /$pattern/ matched $count, need $min ($why)"
    failures=$((failures + 1))
  fi
done < "$manifest"
if (( failures > 0 )); then
  echo "$failures of $checked Prairie invariants failed. If an upstream sync dropped this code, restore it; do not delete the invariant." >&2
  exit 1
fi
echo "All $checked Prairie invariants hold."
