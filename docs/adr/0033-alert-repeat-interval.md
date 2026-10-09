# 33. Ongoing failures repeat on a configurable interval

- Status: Accepted
- Date: 2026-10-08

## Context

A failure that persists (a storage down, backups failing) re-notifies every 24 hours,
hardcoded, and nothing says when it clears.

## Options

1. A setting, defaulting to today's 24 hours, plus a notice on recovery.
2. Escalating backoff (1h, 6h, 24h, then daily).
3. Keep 24 hours fixed.

## Decision

Option 1.

## Consequences

- `ALERT_REPEAT_INTERVAL` (`6h`, `24h`, `0` for never) sets the repeat; the default is 24h.
- One recovery notification is sent when the condition clears.
