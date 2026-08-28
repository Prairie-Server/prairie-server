#!/usr/bin/env bash
# Print golangci-lint package patterns for the current CI scope.
#
# Usage:
#   scripts/ci-go-lint-packages.sh [git-base-ref]
#
# Environment:
#   CI_GO_LINT_FULL=1  force full-module package list (push to main / workflow_dispatch)
#
# On PRs, prefers packages that contain changed .go files so analysis stays
# proportional to the diff. Falls back to a full-module package list when the
# module graph, lint config, or a large surface area changed. Workflow / CI
# script-only edits do not force a full-module lint (the job may still run via
# path filters, but exits quickly with zero packages).
#
# Importer compile coverage for unchanged dependents lives in CI as
# `go build ./...` on the Go tests + coverage job — scoped lint alone cannot
# see reverse dependents without ballooning analysis time.

set -euo pipefail

base="${1:-}"

list_all_packages() {
	# Exclude nested replace module; golangci already skips web/ via config.
	go list -f '{{.Dir}}' ./... | sed "s|^$(pwd)/||" | grep -vE '^(web/|internal/compat/zishang520-webtransport-go(/|$))' | while read -r dir; do
		printf './%s\n' "$dir"
	done | LC_ALL=C sort -u
}

if [[ "${CI_GO_LINT_FULL:-}" == "1" || -z "$base" ]]; then
	list_all_packages
	exit 0
fi

if ! git rev-parse --verify "$base" >/dev/null 2>&1; then
	list_all_packages
	exit 0
fi

mapfile -t changed < <(git diff --name-only --diff-filter=ACMR "$base"...HEAD || true)

if [[ ${#changed[@]} -eq 0 ]]; then
	# Empty triple-dot (shouldn't happen on a real PR with go=true).
	exit 0
fi

for path in "${changed[@]}"; do
	case "$path" in
	# Module / lint config changes can affect every package.
	# CI workflow/script edits alone do not — those are validated by running the
	# job, and forcing a full scan made every CI-tuning PR pay ~3m of analysis.
	go.mod | go.sum | .golangci.yml)
		list_all_packages
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
	# Workflow / coverage-list / script-doc change with no .go edits.
	exit 0
fi

if [[ "$go_files" -gt 200 || ${#dirs[@]} -gt 40 ]]; then
	list_all_packages
	exit 0
fi

packages=()
for dir in "${!dirs[@]}"; do
	packages+=("./${dir}")
done

printf '%s\n' "${packages[@]}" | LC_ALL=C sort -u
