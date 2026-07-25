#!/usr/bin/env bash
# Follow air build output in a tmux session until the build completes.
# Usage: dev-follow-build.sh [session-name] [timeout-seconds]
# Exits 0 on success, 1 on build failure, 2 on timeout.
set -euo pipefail

SESSION="${1:-prairie-dev}"
TIMEOUT="${2:-180}"
LOG="/tmp/prairie-air-build.log"

if ! tmux has-session -t "$SESSION" 2>/dev/null; then
    echo "error: no tmux session '$SESSION'" >&2
    exit 1
fi

# Pipe new tmux pane output to a log file
: > "$LOG"
tmux pipe-pane -t "$SESSION" "cat >> $LOG"
trap 'tmux pipe-pane -t "$SESSION" 2>/dev/null || true' EXIT

# Follow the log, printing each line, until we see a build result
RESULT=2
while IFS= read -r line; do
    printf '%s\n' "$line"
    case "$line" in
        *"running..."*)
            RESULT=0; break ;;
        *"failed to build"*)
            RESULT=1; break ;;
    esac
done < <(timeout "$TIMEOUT" tail -f "$LOG")

case $RESULT in
    0) printf '\n\033[32m==> Build succeeded, server is running.\033[0m\n' ;;
    1) printf '\n\033[31m==> Build FAILED.\033[0m\n' ;;
    2) printf '\n\033[33m==> Timed out after %ds waiting for build result.\033[0m\n' "$TIMEOUT" ;;
esac

exit $RESULT
