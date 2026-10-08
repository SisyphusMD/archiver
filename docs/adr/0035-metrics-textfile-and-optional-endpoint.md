# 35. Metrics go to a Prometheus textfile, with an optional endpoint

- Status: Accepted
- Date: 2026-10-08

## Context

Backup health over time (last success, durations, bytes, copy lag, drill results) is in
logs and status only, so it cannot be graphed or alerted on with existing monitoring.

## Options

1. A Prometheus textfile always, and an HTTP endpoint on request.
2. An HTTP endpoint only.
3. No metrics in v1.

## Decision

Option 1.

## Consequences

- Archiver keeps a Prometheus textfile in the logs volume, for node-exporter's textfile
  collector: last success time per service and target, run durations, bytes uploaded,
  revisions, copy lag, drill results, backup health.
- `METRICS_PORT` opts into a `/metrics` HTTP endpoint serving the same; without it nothing
  listens.
