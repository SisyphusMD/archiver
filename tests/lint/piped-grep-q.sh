#!/usr/bin/env bash
# No test may pipe into `grep -q`. grep -q exits at its first match, so the writer can take
# SIGPIPE on its next write, and under pipefail the pipeline then fails although the line
# was found (restore-drill.sh's named-status check failed CI this way). `grep PATTERN
# >/dev/null` reads to the end and keeps the same exit status.
#
#   bash tests/lint/piped-grep-q.sh

set -euo pipefail

if hits=$(git grep -n -E '(^|[^|])\|[[:space:]]*grep[[:space:]]+-[a-zA-Z]*q' -- tests); then
  printf '%s\n' "$hits" >&2
  echo "pipe into grep PATTERN >/dev/null instead of grep -q (SIGPIPE under pipefail)" >&2
  exit 1
fi
