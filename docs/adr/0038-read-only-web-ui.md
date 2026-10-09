# 38. An opt-in, read-only web page

- Status: Accepted
- Date: 2026-10-08

## Context

Status, backup health and recent runs are seen through `archiver status`, logs, metrics or
notifications. A page would show them at a glance (Cody's idea, 2026-09-30), at the cost of
a listening surface.

## Options

1. In v1, opt-in and read-only, behind the user's proxy.
2. After v1, with status JSON and metrics meanwhile.
3. Not at all.

## Decision

Option 1.

## Consequences

- `WEB_PORT` enables it; nothing listens otherwise.
- One page: backup health, the last run per service and per target, copy workers, drill
  results, the last 200 log lines. Nothing on it changes anything, and secrets never
  appear; it shows what `archiver status` and the metrics hold.
- It has no login of its own and is meant to sit behind a reverse proxy with
  authentication; the docs say so and a startup log line warns when it is on.
