#!/usr/bin/env bash
# Print golangci-lint package patterns for the current CI scope.
#
# Usage:
#   scripts/ci-go-lint-packages.sh [git-base-ref]
#
# Environment:
#   CI_GO_LINT_FULL=1  force ./... (push to main / workflow_dispatch)
#
# On PRs, prefers packages that contain changed .go files so analysis stays
# proportional to the diff. Falls back to ./... when the module graph, lint
# config, or a large surface area changed.

set -euo pipefail

base="${1:-}"

if [[ "${CI_GO_LINT_FULL:-}" == "1" || -z "$base" ]]; then
	echo "./..."
	exit 0
fi

if ! git rev-parse --verify "$base" >/dev/null 2>&1; then
	echo "./..."
	exit 0
fi

mapfile -t changed < <(git diff --name-only --diff-filter=ACMR "$base"...HEAD || true)

if [[ ${#changed[@]} -eq 0 ]]; then
	echo "./..."
	exit 0
fi

for path in "${changed[@]}"; do
	case "$path" in
	go.mod | go.sum | .golangci.yml | scripts/ci-go-lint-packages.sh | scripts/ci-go-lint.sh | .github/workflows/ci.yml)
		echo "./..."
		exit 0
		;;
	esac
done

declare -A dirs=()
go_files=0
for path in "${changed[@]}"; do
	case "$path" in
	*.go)
		go_files=$((go_files + 1))
		dir=$(dirname "$path")
		# Nested module vendored via replace — leave it out of default lint scope.
		case "$dir" in
		internal/compat/zishang520-webtransport-go | internal/compat/zishang520-webtransport-go/*)
			continue
			;;
		esac
		dirs["$dir"]=1
		;;
	esac
done

if [[ "$go_files" -eq 0 ]]; then
	# Go-related non-.go change (coverage list, etc.) — nothing to typecheck.
	exit 0
fi

# Large PRs: scoped lint misses cross-package fallout; do a full pass.
if [[ "$go_files" -gt 200 || ${#dirs[@]} -gt 40 ]]; then
	echo "./..."
	exit 0
fi

packages=()
for dir in "${!dirs[@]}"; do
	packages+=("./${dir}/...")
done

# Stable order for logs / cache keys.
printf '%s\n' "${packages[@]}" | LC_ALL=C sort -u
