# 32. Liveness and backup health are separate

- Status: Accepted
- Date: 2026-10-08

## Context

`archiver healthcheck` backs the image's HEALTHCHECK. It fails only for faults a restart or
a person must fix (a dead scheduler, a crash signature); a backup that finished with errors
stays healthy on purpose, since an unhealthy container restarts under a liveness probe and
repeats the failure. That keeps restarts safe but leaves backup trouble invisible to
monitors.

## Options

1. Keep the healthcheck as liveness; add backup health reported separately.
2. Make the container unhealthy when backups fail.
3. Keep things as they are.

## Decision

Option 1 (Cody left the choice to me).

## Consequences

- `archiver healthcheck` stays a liveness check, safe for restart policies.
- Backup health is OK, DEGRADED (a secondary behind or retrying) or FAILING (no good primary
  backup within twice the schedule, the primary down, the kit stale, a drill failed), from
  each target's state. `archiver health --backups` exits by it for monitors such as Uptime
  Kuma, and status's JSON carries it.
- Nothing restarts because a backup failed.
