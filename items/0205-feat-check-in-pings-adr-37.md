# #205 feat: check-in pings (ADR 37)

- Kind: pull, state: closed, by cody, created 2026-10-09T19:30:26-07:00
- Labels: none; milestone: none
- refs/pull/205/head -> main, merged: True

ADR 37.

- internal/checkin.Ping: success GET; failure variant: Uptime Kuma push URLs (/api/push/) get status=down (status=up on success, msg), others /fail on the path (healthchecks.io). 15 s per attempt, 2 retries; errors never carry the URL (it holds the monitor's token). checkin.Latest sends one target's pings in order without blocking (newest waiting status wins).
- CHECKIN_URL: pinged after every backup run: success, failure (including a failed kit step), a run that failed before any service, and a refused run (lock held, restore running). Pings end only on a stop (halt context), never on the pipeline's own context cleanup.
- STORAGE_TARGET_N_CHECKIN_URL: copy workers ping when caught up (failure instead while a check failure stands), on every failed pass, on check pass/fail; inline copies ping each target with its own outcome; standalone maintenance pings on check results and on storages skipped as unreachable/unregistrable.
- Both validated (http(s)). Settings, not secrets. README "Check-ins".
- tests/integration/checkins.sh; 51 integration scripts + e2e pass locally. 6 Codex rounds, last clean.
