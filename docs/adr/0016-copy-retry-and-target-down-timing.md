# 16. Copies run at any hour and retry on a fixed backoff

- Status: Accepted
- Date: 2026-09-30

## Context

A copy worker retries until its target has caught up (ADR 11). It needs retry timing, a point at which an unreachable target counts as down, and a rule for when copies may run.

## Options

1. Copies at any hour; a copy window setting only if daytime upload load becomes a problem.
2. Copies only inside a configured window, such as 22:00 to 08:00.

Down after 2 hours of failures, or after 30 minutes.

## Decision

Option 1. Retries at 1, 5 and 15 minutes, then every 30 minutes until the target is caught up. A target counts as down after 30 minutes of failures.

## Consequences

- A short blip retries within minutes and costs at most one repeated listing.
- Down sends one alert, a reminder every 24 hours while it lasts, and one recovered alert saying how far behind the target was (Phase 6b builds the alerting).
- Retries continue while the target is down; being down changes alerts, not behavior.
- A copy may use the upload link during the day.
