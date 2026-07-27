#!/usr/bin/env bash
# Enforce a minimum Go statement coverage percentage for listed packages.
#
# Usage:
#   scripts/check-go-coverage.sh [coverprofile] [min_percent]
#
# Environment:
#   COVER_PROFILE   coverprofile path (default: coverage.out)
#   COVER_MIN       minimum percent (default: 95)
#   COVER_PACKAGES  newline/space separated package patterns; when set, each
#                   matched package must meet COVER_MIN. When empty, the
#                   aggregate profile total must meet COVER_MIN.
#   COVER_EXCLUDE_FILE_REGEX
#                   optional RE2/Python regex; matching coverprofile file paths
#                   are omitted from totals (e.g. '(/|^)store\\.go$' to skip
#                   PgStore DB code that is impractical to unit-test).

set -euo pipefail

COVER_PROFILE="${1:-${COVER_PROFILE:-coverage.out}}"
COVER_MIN="${2:-${COVER_MIN:-95}}"

if [[ ! -f "$COVER_PROFILE" ]]; then
  echo "error: cover profile not found: $COVER_PROFILE" >&2
  exit 1
fi

python3 - "$COVER_PROFILE" "$COVER_MIN" <<'PY'
import os, re, sys
from collections import defaultdict

profile, minimum = sys.argv[1], float(sys.argv[2])
required = [p.strip() for p in os.environ.get("COVER_PACKAGES", "").split() if p.strip()]
exclude_re = os.environ.get("COVER_EXCLUDE_FILE_REGEX", "").strip()
exclude = re.compile(exclude_re) if exclude_re else None

# mode: set
# file:start.line,end.line numstmt count
stmts = defaultdict(lambda: [0, 0])  # pkg -> [covered, total]

with open(profile) as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("mode:"):
            continue
        # path.go:N.N,N.N NS NC
        m = re.match(r"^(.+\.go):\d+\.\d+,\d+\.\d+ (\d+) (\d+)$", line)
        if not m:
            continue
        path, num_stmt, count = m.group(1), int(m.group(2)), int(m.group(3))
        if exclude is not None and exclude.search(path):
            continue
        # Derive package directory from file path in the profile.
        # Profiles use module-absolute paths like:
        # github.com/prairie-server/prairie-server/internal/livetv/service.go
        pkg = path.rsplit("/", 1)[0]
        stmts[pkg][1] += num_stmt
        if count > 0:
            stmts[pkg][0] += num_stmt

def pct(covered, total):
    return 0.0 if total == 0 else 100.0 * covered / total

def match_packages(pattern):
    """Match package paths by exact suffix so internal/livetv does not also
    pull in internal/livetv/hdhomerun."""
    pat = pattern.rstrip("/")
    matched = {}
    for pkg, v in stmts.items():
        if pkg == pat or pkg.endswith("/" + pat):
            matched[pkg] = v
    return matched

failed = False

if required:
    print(f"Checking per-package coverage (min {minimum:.1f}%):")
    if exclude_re:
        print(f"  (excluding files matching /{exclude_re}/)")
    for pattern in required:
        matched = match_packages(pattern)
        if not matched:
            print(f"  FAIL  {pattern}: no coverage data (package missing or untested)")
            failed = True
            continue
        covered = sum(v[0] for v in matched.values())
        total = sum(v[1] for v in matched.values())
        p = pct(covered, total)
        status = "OK" if p + 1e-9 >= minimum else "FAIL"
        if status == "FAIL":
            failed = True
        print(f"  {status}  {pattern}: {p:.1f}% ({covered}/{total})")
else:
    covered = sum(v[0] for v in stmts.values())
    total = sum(v[1] for v in stmts.values())
    p = pct(covered, total)
    print(f"Aggregate coverage: {p:.1f}% ({covered}/{total}); minimum {minimum:.1f}%")
    if p + 1e-9 < minimum:
        failed = True

if failed:
    sys.exit(1)
print("Coverage gate passed.")
PY
