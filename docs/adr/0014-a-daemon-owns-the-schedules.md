# 14. A Go daemon owns the schedules

- Status: Accepted
- Date: 2026-09-30

## Context

Supercronic starts each scheduled command as a fresh process, and processes coordinate only through lock files. The copy workers of ADR 11 must run between backups, react when local's revisions change, and retry on their own timers, which no cron entry can express.

## Options

1. A long-lived Go process (the container's main process) that runs the backup, maintenance, and one worker per target; the CLI asks it to act over a local socket.
2. Keep supercronic and coordinate the workers through files and extra cron entries.

## Decision

Option 1.

## Consequences

- Supercronic leaves the image; the daemon parses `BACKUP_SCHEDULE` and `MAINTENANCE_SCHEDULE`, and an invalid schedule still fails container start.
- `archiver backup`, `stop`, `pause`, `resume` and `status` talk to the daemon when one runs.
- Locks become kernel file locks, released when their holder dies, so a restart needs no stale-lock cleanup.
- Correctness never depends on the daemon's memory: after a restart each worker recomputes what its target needs from the storages.
- Without a daemon (a Kubernetes Job, an external scheduler), `archiver run backup` performs one pass in a single process and exits with the result.
