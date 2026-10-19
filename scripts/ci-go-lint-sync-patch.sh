#!/usr/bin/env bash
# Print a patch of the Go lines an upstream sync PR adds relative to BOTH
# parents of its sync merge: Prairie's base branch and the Silo upstream commit
# it merged. golangci-lint --new-from-patch then reports only those lines.
#
# Why: a sync PR diffs every upstream line Prairie did not have yet against
# origin/main, so a plain --new-from-merge-base gate re-reports upstream's
# whole lint backlog (upstream gates each of its own PRs on changed lines
# only, so findings on lines a later upstream PR left alone are never shown
# again there). Lines that exist verbatim upstream or on main were gated
# where they came from; what is new to both is the merge resolution and the
# Prairie commits on top of it, and that stays strict.
#
# Usage: scripts/ci-go-lint-sync-patch.sh <base-ref> [pr-head]
# Exits 3 when pr-head (default HEAD) holds no merge with a parent outside
# base-ref, so the caller can fall back to the ordinary gate.

set -euo pipefail

base="$1"
head="${2:-HEAD}"

upstream=""
while read -r merge; do
	for parent in $(git rev-list --no-walk --parents "$merge" | cut -d' ' -f3-); do
		if ! git merge-base --is-ancestor "$parent" "$base" 2>/dev/null; then
			upstream="$parent"
			break 2
		fi
	done
done < <(git rev-list --merges --first-parent "$base..$head")

if [[ -z "$upstream" ]]; then
	echo "ci-go-lint-sync-patch: no sync merge in $base..$head" >&2
	exit 3
fi
echo "ci-go-lint-sync-patch: lines new to both $base and upstream parent $upstream" >&2

# added_lines REV: "path<TAB>line" for every line HEAD adds relative to REV.
added_lines() {
	git diff --no-color --no-ext-diff --no-renames -U0 "$1" HEAD -- '*.go' | awk '
		/^\+\+\+ / { file = substr($0, 7); next }
		/^@@ / {
			split($3, range, ",")
			start = substr(range[1], 2) + 0
			count = (length(range) > 1) ? range[2] + 0 : 1
			for (i = 0; i < count; i++) print file "\t" start + i
		}'
}

LC_ALL=C comm -12 <(added_lines "$base" | LC_ALL=C sort -u) <(added_lines "$upstream" | LC_ALL=C sort -u) |
	LC_ALL=C sort -t$'\t' -k1,1 -k2,2n |
	awk -F'\t' '
		$1 != file {
			file = $1
			printf "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", file, file, file, file
			n = 0
			delete lines
			while ((getline text < file) > 0) lines[++n] = text
			close(file)
		}
		{ printf "@@ -0,0 +%d,1 @@\n+%s\n", $2, lines[$2] }'
