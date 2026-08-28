#!/usr/bin/env bash
# Run golangci-lint with CI-appropriate package scope (optionally sharded).
#
# Usage:
#   scripts/ci-go-lint.sh [git-base-ref]
#
# Environment:
#   CI_GO_LINT_FULL=1
#   CI_GO_LINT_SHARD / CI_GO_LINT_SHARDS
#   CI_GO_LINT_NEW_FROM_REV=…
#   CI_GO_LINT_NEW_FROM_MERGE_BASE=…

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

base="${1:-}"
mapfile -t packages < <(CI_GO_LINT_FULL="${CI_GO_LINT_FULL:-}" \
	CI_GO_LINT_SHARD="${CI_GO_LINT_SHARD:-}" \
	CI_GO_LINT_SHARDS="${CI_GO_LINT_SHARDS:-}" \
	"$root/scripts/ci-go-lint-packages.sh" "$base")

if [[ ${#packages[@]} -eq 0 ]]; then
	echo "No Go packages in this lint shard/scope; skipping."
	exit 0
fi

args=(--timeout=5m)
if [[ -n "${CI_GO_LINT_NEW_FROM_MERGE_BASE:-}" ]]; then
	args+=(--new-from-merge-base="${CI_GO_LINT_NEW_FROM_MERGE_BASE}")
elif [[ -n "${CI_GO_LINT_NEW_FROM_REV:-}" ]]; then
	args+=(--new-from-rev="${CI_GO_LINT_NEW_FROM_REV}")
fi

echo "golangci-lint run ${args[*]} (${#packages[@]} packages)"
printf '  %s\n' "${packages[@]}"
exec golangci-lint run "${args[@]}" "${packages[@]}"
