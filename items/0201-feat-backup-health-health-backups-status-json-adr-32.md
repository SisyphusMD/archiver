# #201 feat: backup health, health --backups, status --json (ADR 32)

- Kind: pull, state: closed, by cody, created 2026-10-09T16:02:52-07:00
- Labels: none; milestone: none
- refs/pull/201/head -> main, merged: True

ADR 32: liveness and backup health separate.

- Pipeline records each service's outcome in logs/.backup-state.json, keyed by absolute directory (same-named dirs are separate outcomes), after the post hook; early rejections (unsafe hook, unreadable filters, legacy hook file, missing dir) record failed. A failed state write is its own failing incident per service.
- internal/backuphealth.Compute: FAILING when two scheduled backups (counted on the cron itself, so weekday-only schedules are fine at weekends) have come since a service's last good backup, its last backup failed/skipped, SERVICE_DIRECTORIES is unset or an entry matches nothing, state or incidents are unreadable, or a failing incident is open (backup-aborted: a run that failed before any service; backup-state; backup-skipped; kit; drill*; log:*). DEGRADED: other incidents (run-level backup e.g. inline copy failure, copy/check/mirror, maintenance), workers retrying or behind longer than a backup interval (new persisted BehindSince), never backed up yet.
- `archiver health --backups` exits 0/1/2; `archiver status` shows it first; `archiver status --json` carries backup health, runs, services, copies, drills, storages, incidents, envelope.
- healthcheck unchanged (liveness).
- tests/integration/backup-health.sh; unit tests; 48 integration scripts + e2e pass locally. 10 Codex rounds, last clean.
