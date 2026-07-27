#!/usr/bin/env bash
# Print golangci-lint package patterns for the current CI scope.
#
# Usage:
#   scripts/ci-go-lint-packages.sh [git-base-ref]
#
# Environment:
#   CI_GO_LINT_FULL=1     force ./... (push to main / workflow_dispatch)
#   CI_GO_LINT_SHARD=i    zero-based shard index (optional)
#   CI_GO_LINT_SHARDS=n   shard count (optional; with SHARD, emit every n-th pkg)
#
# On PRs, prefers packages that contain changed .go files so analysis stays
# proportional to the diff. Falls back to a full-module package list when the
# module graph, lint config, or a large surface area changed. Workflow-only
# edits do not force a full-module lint (the job may still run via path
# filters, but exits quickly).
#
# Importer compile coverage for unchanged dependents lives in CI as
# `go build ./...` on the Go tests + coverage job — scoped lint alone cannot
# see reverse dependents without ballooning analysis time.

set -euo pipefail

base="${1:-}"

emit_packages() {
	local -a pkgs=("$@")
	local shard="${CI_GO_LINT_SHARD:-}"
	local shards="${CI_GO_LINT_SHARDS:-}"

	if [[ -z "$shard" || -z "$shards" || "$shards" -le 1 ]]; then
		printf '%s\n' "${pkgs[@]}"
		return 0
	fi
	if [[ "$shard" -ge "$shards" ]]; then
		echo "error: CI_GO_LINT_SHARD=$shard >= CI_GO_LINT_SHARDS=$shards" >&2
		exit 1
	fi

	local i=0
	local p
	for p in "${pkgs[@]}"; do
		if (( i % shards == shard )); then
			printf '%s\n' "$p"
		fi
		i=$((i + 1))
	done
}

list_all_packages() {
	# One pattern per top-level package dir under the module (stable sharding).
	# Exclude nested replace module and generated/web trees golangci already skips.
	go list -f '{{.Dir}}' ./... | sed "s|^$(pwd)/||" | grep -vE '^(web/|internal/compat/zishang520-webtransport-go(/|$))' | while read -r dir; do
		printf './%s\n' "$dir"
	done | LC_ALL=C sort -u
}

if [[ "${CI_GO_LINT_FULL:-}" == "1" || -z "$base" ]]; then
	mapfile -t all < <(list_all_packages)
	emit_packages "${all[@]}"
	exit 0
fi

if ! git rev-parse --verify "$base" >/dev/null 2>&1; then
	mapfile -t all < <(list_all_packages)
	emit_packages "${all[@]}"
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
	go.mod | go.sum | .golangci.yml | scripts/ci-go-lint-packages.sh | scripts/ci-go-lint.sh)
		mapfile -t all < <(list_all_packages)
		emit_packages "${all[@]}"
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
	mapfile -t all < <(list_all_packages)
	emit_packages "${all[@]}"
	exit 0
fi

packages=()
for dir in "${!dirs[@]}"; do
	packages+=("./${dir}")
done

mapfile -t sorted < <(printf '%s\n' "${packages[@]}" | LC_ALL=C sort -u)
emit_packages "${sorted[@]}"
