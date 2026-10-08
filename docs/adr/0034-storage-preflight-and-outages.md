# 34. Storages are probed before use, and outages are handled where they happen

- Status: Accepted
- Date: 2026-10-08

## Context

A copy or maintenance against an unreachable storage fails only after duplicacy's own
retries, with a long error. A down primary means no new backups at all, yet reads like any
other failure. A storage can also go down after a run has started.

## Options

1. Probe each storage before use and make a down primary distinct and loud.
2. Only make the down primary distinct.
3. Neither.

## Decision

Option 1, and handle storages that go down mid-run (Cody's addition).

## Consequences

- Before a copy, check or prune, a quick read-only probe confirms the storage is reachable
  and its credentials work; an unreachable secondary is skipped with one line and retried
  later, and every worker retry probes first, so a storage still down costs seconds.
- An unreachable primary fails the backup at once with a distinct PRIMARY DOWN notification
  at a higher priority, and backup health shows FAILING.
- If a service's backup fails mid-run, the primary is probed again; when it is down, no
  further service starts (each is reported skipped: primary down), one PRIMARY DOWN
  notification is sent, and post-backup hooks still run for services whose pre hook ran.
- A storage lost during maintenance fails its own check or prune; the others continue.
- A storage that recovers gets one recovery notification (ADR 33).
