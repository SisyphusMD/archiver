# 52. Each drill kind has its own schedule, metrics and an opt-out from notifications

- Status: Accepted
- Date: 2026-10-13

## Context

With restore drills, deep checks and kit drills (ADRs 28, 50, 51), one schedule no longer
fits: a full offsite check and a small local drill differ by days. The NAS's cluster
watched its old drill container's own metrics, and both it and Archiver can alert.

## Decision

- Each kind has a cron schedule. A run that fails or is interrupted stays due and runs
  again at the next scheduled time until one passes; every run has a time limit.
- Archiver exports, per kind (and storage and service where they apply), the last run, its
  result, its duration and the last success on `METRICS_PORT`, one naming for every kind.
  Deployments that watched the old NAS drill container rewrite their rules to these.
- Drill failures notify by default, repeating on `ALERT_REPEAT_INTERVAL` as today.
  `NOTIFY_ON` gains a way to leave drill results out, for a deployment that alerts from the
  metrics instead.

## Consequences

- No week-of-month windows or monthly failure caps; cron expresses the schedule.
- Nobody loses drill alerts unless they turn them off.
