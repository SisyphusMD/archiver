# #278 test: never pipe into grep -q

- Kind: pull, state: closed, by cody, created 2026-10-24T08:49:29-07:00
- Labels: none; milestone: none
- refs/pull/278/head -> main, merged: True

Under pipefail, `cmd | grep -q PATTERN` can fail although PATTERN is present: grep exits at its first match and the writer takes SIGPIPE. restore-drill.sh:142 failed the Round-trip job on #264 this way. Every piped grep -q in tests/ (36) becomes `grep PATTERN >/dev/null`, and tests/lint/piped-grep-q.sh runs as a CI job gating the build jobs so the pattern cannot come back.
