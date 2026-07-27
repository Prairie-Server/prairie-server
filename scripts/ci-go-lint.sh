#!/usr/bin/env bash
# Run golangci-lint with CI-appropriate package scope.
#
# Usage:
#   scripts/ci-go-lint.sh [git-base-ref]
#
# Environment:
#   CI_GO_LINT_FULL=1           force full-module lint
#   CI_GO_LINT_NEW_FROM_REV=…   --new-from-rev value (push)
#   CI_GO_LINT_NEW_FROM_MERGE_BASE=…  --new-from-merge-base value (PR)

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

base="${1:-}"
mapfile -t packages < <(CI_GO_LINT_FULL="${CI_GO_LINT_FULL:-}" "$root/scripts/ci-go-lint-packages.sh" "$base")

if [[ ${#packages[@]} -eq 0 ]]; then
	echo "No Go packages changed; skipping golangci-lint."
	exit 0
fi

args=(--timeout=5m)
if [[ -n "${CI_GO_LINT_NEW_FROM_MERGE_BASE:-}" ]]; then
	args+=(--new-from-merge-base="${CI_GO_LINT_NEW_FROM_MERGE_BASE}")
elif [[ -n "${CI_GO_LINT_NEW_FROM_REV:-}" ]]; then
	args+=(--new-from-rev="${CI_GO_LINT_NEW_FROM_REV}")
fi

echo "golangci-lint run ${args[*]} ${packages[*]}"
exec golangci-lint run "${args[@]}" "${packages[@]}"
