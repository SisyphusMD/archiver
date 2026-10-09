# 37. Check-in pings, overall and per target

- Status: Accepted
- Date: 2026-10-08

## Context

Notifications and backup health come from Archiver itself, so an Archiver that is dead or
wedged says nothing. A dead man's switch catches that: Archiver calls a URL after each good
run, and an outside monitor alerts when the calls stop.

## Options

1. One URL after each successful backup run, and one per target.
2. One URL only.
3. None in v1.

## Decision

Option 1.

## Consequences

- `CHECKIN_URL` is called after each successful backup run (an Uptime Kuma push monitor,
  healthchecks.io).
- `STORAGE_TARGET_N_CHECKIN_URL` is called when that target is caught up: copy done or
  check passed.
- On failure, the URL's `/fail` variant is called where the service has one, so the alert
  comes at once rather than at the timeout.
