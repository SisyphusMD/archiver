# #187 test: here-strings instead of pipes into grep -q (SIGPIPE flake)

- Kind: pull, state: closed, by cody, created 2026-10-08T16:57:37-07:00
- Labels: none; milestone: none
- refs/pull/187/head -> main, merged: True

PR #182 failed host-driven integration on invalid-cron.sh: `docker logs X | grep -q msg` under pipefail, where grep exits at its first match and docker logs dies of SIGPIPE, so the step reported the message missing. Every top-level pipe into grep -q in the integration scripts now reads a here-string. All five touched scripts pass locally.
