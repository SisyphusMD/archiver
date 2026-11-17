# #44 feat: swap Debian cron for supercronic (drop SETGID, add tzdata)

- Kind: pull, state: closed, by cody, created 2026-07-06T20:27:14-07:00
- Labels: none; milestone: none
- refs/pull/44/head -> main, merged: True

Phase 2 of the overhaul: replace the in-container Debian `cron` daemon with [supercronic](https://github.com/aptible/supercronic) and drop the `SETGID` capability.

## Why
`cron` forks with `setgid` to exec jobs, which is why the hardened `cap_drop: ALL` template had to add `SETGID` back. supercronic runs jobs as the container user, so `SETGID` is no longer needed and is removed from the documented cap set (now `DAC_OVERRIDE` + `CHOWN` + `FOWNER`). supercronic also passes the container environment straight through (Debian cron scrubs it), which unblocks the later env-native-config phase.

## What
- **Dockerfile**: drop `cron`, add `tzdata`, download supercronic (Renovate-pinned via `# renovate:` comment, tracked by the existing `dockerfileVersions` preset).
- **docker-entrypoint.sh**: cron block → `supercronic -test` (fail-fast on a bad `CRON_SCHEDULE`) + backgrounded `supercronic -passthrough-logs & wait` — **not** `exec`, so the SIGTERM graceful-stop trap and log-tailer still work.
- **Caps**: `SETGID` removed from `compose.yaml`, `README.md` (both cap blocks + table + note + daemon-mode line), the round-trip doc comment, and the CI integration invocation.
- **New test** `tests/integration/scheduler.sh` + a step in the `integration` CI job: fires a scheduled job under `cap_drop: ALL` with **no `SETGID`** and asserts it ran.

## Interface unchanged
`CRON_SCHEDULE` (standard 5-field) and `TZ` behave exactly as before — old compose files with `SETGID` still run fine (the cap just becomes unused). Only edge: supercronic rejects `@reboot`/`@midnight`/`@every` (never documented), and the new `-test` makes an unsupported schedule fail loudly at startup instead of silently never firing.

## Validation (local, amd64)
Image builds (supercronic present, cron absent); round-trip passes **without SETGID** (content + mode + UID/GID preserved); scheduler smoke passes **without SETGID**; shellcheck clean; bats 23/23.

Version bump is intentionally **not** in this PR — `release.yml` (dispatch `bump=patch` → 0.8.12) owns the CHANGELOG promotion and image-tag bump.
