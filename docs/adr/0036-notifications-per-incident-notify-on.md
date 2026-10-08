# 36. Notifications are per incident, chosen with NOTIFY_ON

- Status: Accepted
- Date: 2026-10-08

## Context

Every ERROR log line sends a notification, so one failing backup can send several, and
there is no way to ask for routine news (backup done) or to send different things to
different destinations. v1 adds Apprise and ntfy beside Pushover as first-class
destinations (Cody, 2026-10-04). Naming a notification setting after the log levels would
blur two different things.

## Options

1. Events always notified on errors, the rest as per-event toggles.
2. Levels named after the log levels (error, warning, info).
3. Levels in their own words: failures, problems, everything.

## Decision

Option 3.

## Consequences

- `NOTIFY_ON` is `failures` (the default: backup, copy, restore or drill failed, primary
  down, a storage down, the kit stale), `problems` (also a secondary retrying or behind, a
  drill skipped for space, the envelope check due, a disk filling) or `everything` (also
  backup done, copy caught up, maintenance done, a daily digest).
- Each destination can override it (`APPRISE_NOTIFY_ON`, `NTFY_NOTIFY_ON`,
  `PUSHOVER_NOTIFY_ON`); failures always go to every destination.
- A notification is one per incident, not one per ERROR line; an identical one repeats only
  on ADR 33's interval, and its recovery notice goes wherever it went.
- Log lines keep INFO, WARNING and ERROR, which no longer decide notifications.
- `archiver notify test` sends one message per setting to every destination.
